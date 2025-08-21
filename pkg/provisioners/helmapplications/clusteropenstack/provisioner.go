/*
Copyright 2022-2024 EscherCloud.
Copyright 2024-2025 the Unikorn Authors.

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

package clusteropenstack

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"

	semver "github.com/Masterminds/semver/v3"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/core/pkg/constants"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	kubernetesprovisioners "github.com/unikorn-cloud/kubernetes/pkg/provisioners"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"

	"k8s.io/utils/ptr"
)

var (
	ErrReference = errors.New("reference error")
)

// Provisioner encapsulates control plane provisioning.
type Provisioner struct {
	options *kubernetesprovisioners.ClusterOpenstackOptions
	// clusterManagerPrefix contains the IP address prefix to add
	// to the cluster firewall if, required.
	clusterManagerPrefix string
}

// New returns a new initialized provisioner object.
func New(getApplication application.GetterFunc, options *kubernetesprovisioners.ClusterOpenstackOptions, clusterManagerPrefix string) *application.Provisioner {
	provisioner := &Provisioner{
		options:              options,
		clusterManagerPrefix: clusterManagerPrefix,
	}

	return application.New(getApplication).WithGenerator(provisioner).AllowDegraded()
}

// Ensure the Provisioner interface is implemented.
var _ application.ReleaseNamer = &Provisioner{}
var _ application.ValuesGenerator = &Provisioner{}
var _ application.PostProvisionHook = &Provisioner{}

// getFlavor looks up a flavor.
func (p *Provisioner) getFlavor(flavorID string) (*regionapi.Flavor, error) {
	callback := func(flavor regionapi.Flavor) bool {
		return flavor.Metadata.Id == flavorID
	}

	index := slices.IndexFunc(p.options.Flavors, callback)
	if index < 0 {
		return nil, fmt.Errorf("%w: unable to find requested flavor %s", ErrReference, flavorID)
	}

	return &p.options.Flavors[index], nil
}

// generateMachineHelmValues translates the API's idea of a machine into what's
// expected by the underlying Helm chart.
func (p *Provisioner) generateMachineHelmValues(machine *unikornv1core.MachineGeneric, controlPlane bool) (map[string]any, error) {
	flavor, err := p.getFlavor(machine.FlavorID)
	if err != nil {
		return nil, err
	}

	// Translate from flavor ID to flavor name, because CAPO cannot accept one...
	// https://github.com/kubernetes-sigs/cluster-api-provider-openstack/pull/2148
	object := map[string]any{
		"imageID":  machine.ImageID,
		"flavorID": flavor.Metadata.Name,
	}

	if controlPlane && p.options.ServerGroupID != nil {
		object["serverGroupID"] = *p.options.ServerGroupID
	}

	if machine.DiskSize != nil {
		disk := map[string]any{
			"size": machine.DiskSize.Value() >> 30,
		}

		object["disk"] = disk
	}

	return object, nil
}

// generateWorkloadPoolHelmValues translates the API's idea of a workload pool into
// what's expected by the underlying Helm chart.
//
//nolint:cyclop
func (p *Provisioner) generateWorkloadPoolHelmValues(cluster *unikornv1.KubernetesCluster, version unikornv1core.SemanticVersion) (map[string]any, error) {
	workloadPools := map[string]any{}

	for i := range cluster.Spec.WorkloadPools.Pools {
		workloadPool := &cluster.Spec.WorkloadPools.Pools[i]

		machine, err := p.generateMachineHelmValues(&workloadPool.MachineGeneric, false)
		if err != nil {
			return nil, err
		}

		object := map[string]any{
			"replicas": workloadPool.Replicas,
			"machine":  machine,
		}

		if cluster.AutoscalingEnabled() && workloadPool.Autoscaling != nil {
			autoscaling, err := p.generateWorkloadPoolSchedulerHelmValues(workloadPool)
			if err != nil {
				return nil, err
			}

			object["autoscaling"] = autoscaling
		}

		if len(workloadPool.Labels) != 0 {
			labels := map[string]any{}

			for key, value := range workloadPool.Labels {
				labels[key] = value
			}

			object["labels"] = labels
		}

		if len(workloadPool.Files) != 0 {
			files := make([]any, len(workloadPool.Files))

			for i, file := range workloadPool.Files {
				files[i] = map[string]any{
					"path":    file.Path,
					"content": base64.StdEncoding.EncodeToString(file.Content),
				}
			}

			object["files"] = files
		}

		// From 0.5.8 onward we are able to turn parallel image pulls on.
		from := &unikornv1core.SemanticVersion{
			Version: *semver.MustParse("0.5.8"),
		}

		if version.Compare(from) >= 0 {
			object["kubelet"] = map[string]any{
				"serializeImagePulls":   false,
				"maxParallelImagePulls": 3,
			}
		}

		workloadPools[workloadPool.Name] = object
	}

	return workloadPools, nil
}

// generateWorkloadPoolSchedulerHelmValues translates from Kubernetes API scheduling
// parameters into ones acceptable by Helm.
func (p *Provisioner) generateWorkloadPoolSchedulerHelmValues(pool *unikornv1.KubernetesWorkloadPoolSpec) (map[string]any, error) {
	// When scaler from zero is enabled, you need to provide CPU and memory hints,
	// the autoscaler cannot guess the flavor attributes.
	flavor, err := p.getFlavor(pool.FlavorID)
	if err != nil {
		return nil, err
	}

	scheduling := map[string]any{
		"cpu":    flavor.Spec.Cpus,
		"memory": fmt.Sprintf("%dG", flavor.Spec.Memory),
	}

	// Add in labels for GPUs so the autoscaler can correctly scale when more GPUs
	// are requested.  To derive this value, each vendor will probably provide a
	// node-labeller that you should install on the system.  It should then appear
	// in the status.capacity map.
	if flavor.Spec.Gpu != nil {
		var t string

		switch flavor.Spec.Gpu.Vendor {
		case regionapi.NVIDIA:
			t = "nvidia.com/gpu"
		case regionapi.AMD:
			t = "amd.com/gpu"
		default:
			return nil, fmt.Errorf("%w: unhandled gpu vendor case %s", ErrReference, flavor.Spec.Gpu.Vendor)
		}

		gpu := map[string]any{
			"type":  t,
			"count": flavor.Spec.Gpu.LogicalCount,
		}

		scheduling["gpu"] = gpu
	}

	values := map[string]any{
		"limits": map[string]any{
			"minReplicas": pool.Autoscaling.MinimumReplicas,
			"maxReplicas": pool.Replicas,
		},
		"scheduler": scheduling,
	}

	return values, nil
}

func (p *Provisioner) generateNetworkValues(cluster *unikornv1.KubernetesCluster) any {
	nameservers := make([]any, len(cluster.Spec.Network.DNSNameservers))

	for i, nameserver := range cluster.Spec.Network.DNSNameservers {
		nameservers[i] = nameserver.String()
	}

	values := map[string]any{
		"nodeCIDR": cluster.Spec.Network.NodeNetwork.String(),
		"serviceCIDRs": []any{
			cluster.Spec.Network.ServiceNetwork.String(),
		},
		"podCIDRs": []any{
			cluster.Spec.Network.PodNetwork.String(),
		},
		"dnsNameservers": nameservers,
	}

	if p.options.ProviderNetwork != nil {
		values["provider"] = map[string]any{
			"networkID": *p.options.ProviderNetwork.NetworkID,
			"subnetID":  *p.options.ProviderNetwork.SubnetID,
		}
	}

	if p.options.SSHKeyName != nil {
		values["securityGroupRules"] = []any{
			map[string]any{
				"name":         "ssh-ingress",
				"direction":    "ingress",
				"etherType":    "IPv4",
				"protocol":     "TCP",
				"portRangeMin": 22,
				"portRangeMax": 22,
			},
		}
	}

	return values
}

// Generate implements the application.Generator interface.
func (p *Provisioner) Values(ctx context.Context, version unikornv1core.SemanticVersion) (any, error) {
	//nolint:forcetypeassert
	cluster := application.FromContext(ctx).(*unikornv1.KubernetesCluster)

	workloadPools, err := p.generateWorkloadPoolHelmValues(cluster, version)
	if err != nil {
		return nil, err
	}

	openstackValues := map[string]any{
		"cloud":      p.options.Cloud,
		"cloudsYAML": p.options.CloudConfig,
	}

	if p.options.ExternalNetworkID != nil {
		openstackValues["externalNetworkID"] = *p.options.ExternalNetworkID
	}

	if p.options.SSHKeyName != nil {
		openstackValues["sshKeyName"] = *p.options.SSHKeyName
	}

	labels, err := cluster.ResourceLabels()
	if err != nil {
		return nil, err
	}

	// These must have parity with what's defined by the API to make
	// cross referencing between unikorn and openstack logging easier.
	serverMetadata := map[string]any{
		"clusterKind":    "kubernetes",
		"clusterID":      cluster.Name,
		"projectID":      labels[constants.ProjectLabel],
		"organizationID": labels[constants.OrganizationLabel],
		"regionID":       cluster.Spec.RegionID,
	}

	machine, err := p.generateMachineHelmValues(&cluster.Spec.ControlPlane, true)
	if err != nil {
		return nil, err
	}

	// TODO: generate types from the Helm values schema.
	values := map[string]any{
		"version":   ptr.To(cluster.Spec.Version.Original()),
		"openstack": openstackValues,
		"cluster": map[string]any{
			"taints": []any{
				// TODO: This is deprecated moving forward as the cilium operator provides these taints.
				//   We can't remove it yet though as it'd break any deployments pre kubernetes-cluster-1.4.0

				// This prevents things like coreDNS from coming up until
				// the CNI is installed.
				map[string]any{
					"key":    "node.cilium.io/agent-not-ready",
					"effect": "NoSchedule",
					"value":  "true",
				},
			},
			"serverMetadata": serverMetadata,
		},
		"controlPlane": map[string]any{
			"replicas": cluster.Spec.ControlPlane.Replicas,
			"machine":  machine,
		},
		"workloadPools": workloadPools,
		"network":       p.generateNetworkValues(cluster),
	}

	if cluster.Spec.API != nil {
		apiValues := map[string]any{}

		if cluster.Spec.API.SubjectAlternativeNames != nil {
			apiValues["certificateSANs"] = cluster.Spec.API.SubjectAlternativeNames
		}

		if cluster.Spec.API.AllowedPrefixes != nil {
			// Add the SNAT IP so CAPI can manage the cluster.
			allowList := []any{
				p.clusterManagerPrefix,
			}

			for _, prefix := range cluster.Spec.API.AllowedPrefixes {
				allowList = append(allowList, prefix.String())
			}

			apiValues["allowList"] = allowList
		}

		values["api"] = apiValues
	}

	return values, nil
}

// ReleaseName implements the application.ReleaseNamer interface.
func (p *Provisioner) ReleaseName(ctx context.Context) string {
	//nolint:forcetypeassert
	cluster := application.FromContext(ctx).(*unikornv1.KubernetesCluster)

	return releaseName(cluster)
}

// PostHook implements the apllication PostProvisionHook interface.
func (p *Provisioner) PostProvision(ctx context.Context) error {
	return p.deleteOrphanedMachineDeployments(ctx)
}
