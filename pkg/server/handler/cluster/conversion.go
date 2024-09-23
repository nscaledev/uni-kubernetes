/*
Copyright 2022-2024 EscherCloud.
Copyright 2024 the Unikorn Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cluster

import (
	"context"
	goerrors "errors"
	"fmt"
	"net/http"
	"slices"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	coreopenapi "github.com/unikorn-cloud/core/pkg/openapi"
	"github.com/unikorn-cloud/core/pkg/server/conversion"
	"github.com/unikorn-cloud/core/pkg/server/errors"
	"github.com/unikorn-cloud/core/pkg/util"
	"github.com/unikorn-cloud/identity/pkg/middleware/authorization"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/openapi"
	"github.com/unikorn-cloud/kubernetes/pkg/server/handler/applicationbundle"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"

	"k8s.io/apimachinery/pkg/api/resource"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrResourceLookup = goerrors.New("could not find the requested resource")
)

// generator wraps up the myriad things we need to pass around as an object
// rather than a whole bunch of arguments.
type generator struct {
	// client allows Kubernetes access.
	client client.Client
	// options allows access to resource defaults.
	options *Options
	// region is a client to access regions.
	region regionapi.ClientWithResponsesInterface
	// namespace the resource is provisioned in.
	namespace string
	// organizationID is the unique organization identifier.
	organizationID string
	// projectID is the unique project identifier.
	projectID string
}

func newGenerator(client client.Client, options *Options, region regionapi.ClientWithResponsesInterface, namespace, organizationID, projectID string) *generator {
	return &generator{
		client:         client,
		options:        options,
		region:         region,
		namespace:      namespace,
		organizationID: organizationID,
		projectID:      projectID,
	}
}

// convertMachine converts from a custom resource into the API definition.
func convertMachine(in *unikornv1.MachineGeneric) *openapi.MachinePool {
	machine := &openapi.MachinePool{
		Replicas: in.Replicas,
		FlavorId: in.FlavorID,
	}

	if in.DiskSize != nil {
		machine.Disk = &openapi.Volume{
			Size: int(in.DiskSize.Value()) >> 30,
		}
	}

	return machine
}

// convertWorkloadPool converts from a custom resource into the API definition.
func convertWorkloadPool(in *unikornv1.KubernetesClusterWorkloadPoolsPoolSpec) openapi.KubernetesClusterWorkloadPool {
	workloadPool := openapi.KubernetesClusterWorkloadPool{
		Name:    in.Name,
		Machine: *convertMachine(&in.KubernetesWorkloadPoolSpec.MachineGeneric),
	}

	if in.KubernetesWorkloadPoolSpec.Labels != nil {
		workloadPool.Labels = &in.KubernetesWorkloadPoolSpec.Labels
	}

	if in.KubernetesWorkloadPoolSpec.Autoscaling != nil {
		workloadPool.Autoscaling = &openapi.KubernetesClusterAutoscaling{
			MinimumReplicas: *in.KubernetesWorkloadPoolSpec.Autoscaling.MinimumReplicas,
		}
	}

	return workloadPool
}

// convertWorkloadPools converts from a custom resource into the API definition.
func convertWorkloadPools(in *unikornv1.KubernetesCluster) []openapi.KubernetesClusterWorkloadPool {
	workloadPools := make([]openapi.KubernetesClusterWorkloadPool, len(in.Spec.WorkloadPools.Pools))

	for i := range in.Spec.WorkloadPools.Pools {
		workloadPools[i] = convertWorkloadPool(&in.Spec.WorkloadPools.Pools[i])
	}

	return workloadPools
}

// convert converts from a custom resource into the API definition.
func convert(in *unikornv1.KubernetesCluster) *openapi.KubernetesClusterRead {
	provisioningStatus := coreopenapi.ResourceProvisioningStatusUnknown

	if condition, err := in.StatusConditionRead(unikornv1core.ConditionAvailable); err == nil {
		provisioningStatus = conversion.ConvertStatusCondition(condition)
	}

	out := &openapi.KubernetesClusterRead{
		Metadata: conversion.ProjectScopedResourceReadMetadata(in, provisioningStatus),
		Spec: openapi.KubernetesClusterSpec{
			RegionId:         in.Spec.RegionID,
			ClusterManagerId: &in.Spec.ClusterManagerID,
			Version:          string(*in.Spec.Version),
			WorkloadPools:    convertWorkloadPools(in),
		},
	}

	return out
}

// uconvertList converts from a custom resource list into the API definition.
func convertList(in *unikornv1.KubernetesClusterList) openapi.KubernetesClusters {
	out := make(openapi.KubernetesClusters, len(in.Items))

	for i := range in.Items {
		out[i] = *convert(&in.Items[i])
	}

	return out
}

// defaultApplicationBundle returns a default application bundle.
func (g *generator) defaultApplicationBundle(ctx context.Context) (*unikornv1.KubernetesClusterApplicationBundle, error) {
	applicationBundles, err := applicationbundle.NewClient(g.client).ListCluster(ctx)
	if err != nil {
		return nil, err
	}

	applicationBundles.Items = slices.DeleteFunc(applicationBundles.Items, func(bundle unikornv1.KubernetesClusterApplicationBundle) bool {
		if bundle.Spec.Preview != nil && *bundle.Spec.Preview {
			return true
		}

		if bundle.Spec.EndOfLife != nil {
			return true
		}

		return false
	})

	if len(applicationBundles.Items) == 0 {
		return nil, errors.OAuth2ServerError("unable to select an application bundle")
	}

	return &applicationBundles.Items[0], nil
}

// defaultControlPlaneFlavor returns a default control plane flavor.  This will be
// one that doesxn't have any GPUs.  The provider ensures the "nost cost-effective"
// comes first.
// TODO: we should allow this to be configured per region.
func (g *generator) defaultControlPlaneFlavor(ctx context.Context, request *openapi.KubernetesClusterWrite) (*regionapi.Flavor, error) {
	resp, err := g.region.GetApiV1OrganizationsOrganizationIDRegionsRegionIDFlavorsWithResponse(ctx, g.organizationID, request.Spec.RegionId)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, errors.OAuth2ServerError("failed to list flavors")
	}

	flavors := *resp.JSON200

	flavors = slices.DeleteFunc(flavors, func(x regionapi.Flavor) bool { return x.Spec.Gpu != nil })

	if len(flavors) == 0 {
		return nil, errors.OAuth2ServerError("unable to select a control plane flavor")
	}

	return &flavors[0], nil
}

// defaultImage returns a default image for either control planes or workload pools
// based on the specified Kubernetes version.
func (g *generator) defaultImage(ctx context.Context, request *openapi.KubernetesClusterWrite, version string) (*regionapi.Image, error) {
	resp, err := g.region.GetApiV1OrganizationsOrganizationIDRegionsRegionIDImagesWithResponse(ctx, g.organizationID, request.Spec.RegionId)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, errors.OAuth2ServerError("failed to list images")
	}

	images := *resp.JSON200

	images = slices.DeleteFunc(images, func(x regionapi.Image) bool {
		return x.Spec.SoftwareVersions == nil || x.Spec.SoftwareVersions.Kubernetes == nil || *x.Spec.SoftwareVersions.Kubernetes != version
	})

	if len(images) == 0 {
		return nil, errors.OAuth2ServerError("unable to select an image")
	}

	return &images[0], nil
}

// generateNetwork generates the network part of a cluster.
func (g *generator) generateNetwork() *unikornv1.KubernetesClusterNetworkSpec {
	// Grab some defaults (as these are in the right format already)
	// the override with anything coming in from the API, if set.
	nodeNetwork := g.options.NodeNetwork
	serviceNetwork := g.options.ServiceNetwork
	podNetwork := g.options.PodNetwork
	dnsNameservers := g.options.DNSNameservers

	network := &unikornv1.KubernetesClusterNetworkSpec{
		NetworkGeneric: unikornv1core.NetworkGeneric{
			NodeNetwork:    &unikornv1core.IPv4Prefix{IPNet: nodeNetwork},
			DNSNameservers: unikornv1core.IPv4AddressSliceFromIPSlice(dnsNameservers),
		},
		ServiceNetwork: &unikornv1core.IPv4Prefix{IPNet: serviceNetwork},
		PodNetwork:     &unikornv1core.IPv4Prefix{IPNet: podNetwork},
	}

	return network
}

// generateMachineGeneric generates a generic machine part of the cluster.
func (g *generator) generateMachineGeneric(ctx context.Context, request *openapi.KubernetesClusterWrite, m *openapi.MachinePool, flavor *regionapi.Flavor) (*unikornv1.MachineGeneric, error) {
	if m.Replicas == nil {
		m.Replicas = util.ToPointer(3)
	}

	image, err := g.defaultImage(ctx, request, request.Spec.Version)
	if err != nil {
		return nil, err
	}

	machine := &unikornv1.MachineGeneric{
		MachineGeneric: unikornv1core.MachineGeneric{
			Replicas: m.Replicas,
			ImageID:  util.ToPointer(image.Metadata.Id),
			FlavorID: &flavor.Metadata.Id,
		},
		// TODO: remove with https://github.com/kubernetes-sigs/cluster-api-provider-openstack/pull/2148
		FlavorName: &flavor.Metadata.Name,
	}

	if m.Disk != nil {
		size, err := resource.ParseQuantity(fmt.Sprintf("%dGi", m.Disk.Size))
		if err != nil {
			return nil, errors.OAuth2InvalidRequest("failed to parse disk size").WithError(err)
		}

		machine.DiskSize = &size
	}

	return machine, nil
}

// generateControlPlane generates the control plane part of a cluster.
func (g *generator) generateControlPlane(ctx context.Context, request *openapi.KubernetesClusterWrite) (*unikornv1.KubernetesClusterControlPlaneSpec, error) {
	// Add in any missing defaults.
	resource, err := g.defaultControlPlaneFlavor(ctx, request)
	if err != nil {
		return nil, err
	}

	machineOptions := &openapi.MachinePool{
		FlavorId: &resource.Metadata.Id,
	}

	machine, err := g.generateMachineGeneric(ctx, request, machineOptions, resource)
	if err != nil {
		return nil, err
	}

	spec := &unikornv1.KubernetesClusterControlPlaneSpec{
		MachineGeneric: *machine,
	}

	return spec, nil
}

// generateWorkloadPools generates the workload pools part of a cluster.
func (g *generator) generateWorkloadPools(ctx context.Context, request *openapi.KubernetesClusterWrite) (*unikornv1.KubernetesClusterWorkloadPoolsSpec, error) {
	workloadPools := &unikornv1.KubernetesClusterWorkloadPoolsSpec{}

	for i := range request.Spec.WorkloadPools {
		pool := &request.Spec.WorkloadPools[i]

		flavor, err := g.lookupFlavor(ctx, request, *pool.Machine.FlavorId)
		if err != nil {
			return nil, err
		}

		machine, err := g.generateMachineGeneric(ctx, request, &pool.Machine, flavor)
		if err != nil {
			return nil, err
		}

		workloadPool := unikornv1.KubernetesClusterWorkloadPoolsPoolSpec{
			KubernetesWorkloadPoolSpec: unikornv1.KubernetesWorkloadPoolSpec{
				Name:           pool.Name,
				MachineGeneric: *machine,
			},
		}

		if pool.Labels != nil {
			workloadPool.Labels = *pool.Labels
		}

		// With autoscaling, we automatically fill in the required metadata from
		// the flavor used in validation, this prevents having to surface this
		// complexity to the client via the API.
		if pool.Autoscaling != nil {
			memory, err := resource.ParseQuantity(fmt.Sprintf("%dGi", flavor.Spec.Memory))
			if err != nil {
				return nil, err
			}

			workloadPool.Autoscaling = &unikornv1.MachineGenericAutoscaling{
				MinimumReplicas: &pool.Autoscaling.MinimumReplicas,
				MaximumReplicas: pool.Machine.Replicas,
				Scheduler: &unikornv1.MachineGenericAutoscalingScheduler{
					CPU:    &flavor.Spec.Cpus,
					Memory: &memory,
				},
			}

			if flavor.Spec.Gpu != nil {
				// TODO: this is needed for scale from zero, at no point do the docs
				// mention AMD...
				t := "nvidia.com/gpu"

				workloadPool.Autoscaling.Scheduler.GPU = &unikornv1.MachineGenericAutoscalingSchedulerGPU{
					Type:  &t,
					Count: &flavor.Spec.Gpu.Count,
				}
			}
		}

		workloadPools.Pools = append(workloadPools.Pools, workloadPool)
	}

	return workloadPools, nil
}

// lookupFlavor resolves the flavor from its name.
// NOTE: It looks like garbage performance, but the provider should be memoized...
func (g *generator) lookupFlavor(ctx context.Context, request *openapi.KubernetesClusterWrite, id string) (*regionapi.Flavor, error) {
	resp, err := g.region.GetApiV1OrganizationsOrganizationIDRegionsRegionIDFlavorsWithResponse(ctx, g.organizationID, request.Spec.RegionId)
	if err != nil {
		return nil, err
	}

	flavors := *resp.JSON200

	index := slices.IndexFunc(flavors, func(flavor regionapi.Flavor) bool {
		return flavor.Metadata.Id == id
	})

	if index < 0 {
		return nil, fmt.Errorf("%w: flavor %s", ErrResourceLookup, id)
	}

	return &flavors[index], nil
}

// installNvidiaOperator installs the nvidia operator if any workload pool flavor
// has a GPU in it.
func (g *generator) installNvidiaOperator(ctx context.Context, request *openapi.KubernetesClusterWrite, cluster *unikornv1.KubernetesCluster) error {
	for _, pool := range request.Spec.WorkloadPools {
		flavor, err := g.lookupFlavor(ctx, request, *pool.Machine.FlavorId)
		if err != nil {
			return err
		}

		if flavor.Spec.Gpu != nil && flavor.Spec.Gpu.Vendor == regionapi.NVIDIA {
			cluster.Spec.Features.NvidiaOperator = util.ToPointer(true)

			return nil
		}
	}

	return nil
}

// installClusterAutoscaler installs the cluster autoscaler if any workload pool has
// autoscaling enabled.
// TODO: probably push this down into the cluster manager.
func installClusterAutoscaler(cluster *unikornv1.KubernetesCluster) {
	for _, pool := range cluster.Spec.WorkloadPools.Pools {
		if pool.Autoscaling != nil {
			cluster.Spec.Features.Autoscaling = util.ToPointer(true)

			return
		}
	}
}

// generate generates the full cluster custom resource.
// TODO: there are a lot of parameters being passed about, we should make this
// a struct and pass them as a single blob.
func (g *generator) generate(ctx context.Context, request *openapi.KubernetesClusterWrite) (*unikornv1.KubernetesCluster, error) {
	kubernetesControlPlane, err := g.generateControlPlane(ctx, request)
	if err != nil {
		return nil, err
	}

	kubernetesWorkloadPools, err := g.generateWorkloadPools(ctx, request)
	if err != nil {
		return nil, err
	}

	applicationBundle, err := g.defaultApplicationBundle(ctx)
	if err != nil {
		return nil, err
	}

	userinfo, err := authorization.UserinfoFromContext(ctx)
	if err != nil {
		return nil, err
	}

	cluster := &unikornv1.KubernetesCluster{
		ObjectMeta: conversion.NewObjectMetadata(&request.Metadata, g.namespace, userinfo.Sub).WithOrganization(g.organizationID).WithProject(g.projectID).Get(),
		Spec: unikornv1.KubernetesClusterSpec{
			RegionID:                     request.Spec.RegionId,
			ClusterManagerID:             *request.Spec.ClusterManagerId,
			Version:                      util.ToPointer(unikornv1core.SemanticVersion(request.Spec.Version)),
			ApplicationBundle:            &applicationBundle.Name,
			ApplicationBundleAutoUpgrade: &unikornv1.ApplicationBundleAutoUpgradeSpec{},
			Network:                      g.generateNetwork(),
			ControlPlane:                 kubernetesControlPlane,
			WorkloadPools:                kubernetesWorkloadPools,
			Features:                     &unikornv1.KubernetesClusterFeaturesSpec{},
		},
	}

	installClusterAutoscaler(cluster)

	if err := g.installNvidiaOperator(ctx, request, cluster); err != nil {
		return nil, err
	}

	return cluster, nil
}
