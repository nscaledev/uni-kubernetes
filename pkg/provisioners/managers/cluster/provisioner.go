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

package cluster

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/pflag"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	coreclient "github.com/unikorn-cloud/core/pkg/client"
	coreconstants "github.com/unikorn-cloud/core/pkg/constants"
	"github.com/unikorn-cloud/core/pkg/manager"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	"github.com/unikorn-cloud/core/pkg/provisioners"
	"github.com/unikorn-cloud/core/pkg/provisioners/concurrent"
	"github.com/unikorn-cloud/core/pkg/provisioners/conditional"
	"github.com/unikorn-cloud/core/pkg/provisioners/remotecluster"
	"github.com/unikorn-cloud/core/pkg/provisioners/serial"
	"github.com/unikorn-cloud/core/pkg/util"
	coreapiutils "github.com/unikorn-cloud/core/pkg/util/api"
	identityclient "github.com/unikorn-cloud/identity/pkg/client"
	identityapi "github.com/unikorn-cloud/identity/pkg/openapi"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/constants"
	"github.com/unikorn-cloud/kubernetes/pkg/internal/applicationbundle"
	kubernetesprovisioners "github.com/unikorn-cloud/kubernetes/pkg/provisioners"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/amdgpuoperator"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/certmanager"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/cilium"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/clusterautoscaler"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/clusterautoscaleropenstack"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/clusteropenstack"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/gatewayapi"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/metricsserver"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/nvidiagpuoperator"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/openstackcloudprovider"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/openstackplugincindercsi"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/vcluster"
	regionclient "github.com/unikorn-cloud/region/pkg/client"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	ErrClusterManager = errors.New("cluster manager lookup failed")

	ErrAnnotation = errors.New("required annotation missing")

	ErrResourceDependency = errors.New("resource dependency error")
)

type ApplicationReferenceGetter struct {
	cluster *unikornv1.KubernetesCluster
}

func newApplicationReferenceGetter(cluster *unikornv1.KubernetesCluster) *ApplicationReferenceGetter {
	return &ApplicationReferenceGetter{
		cluster: cluster,
	}
}

func (a *ApplicationReferenceGetter) getBundle(ctx context.Context) (*unikornv1.KubernetesClusterApplicationBundle, error) {
	namespace, err := coreclient.NamespaceFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: we could cache this, it's from a cache anyway, so quite cheap...
	cli, err := coreclient.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	appclient := applicationbundle.NewClient(cli, namespace)

	bundle, err := appclient.GetKubernetesCluster(ctx, a.cluster.Spec.ApplicationBundle)
	if err != nil {
		return nil, err
	}

	return bundle, nil
}

func (a *ApplicationReferenceGetter) getApplication(ctx context.Context, name string) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	bundle, err := a.getBundle(ctx)
	if err != nil {
		return nil, nil, err
	}

	reference, err := bundle.Spec.GetApplication(name)
	if err != nil {
		return nil, nil, err
	}

	namespace, err := coreclient.NamespaceFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	appkey := client.ObjectKey{
		Namespace: namespace,
		Name:      *reference.Name,
	}

	cli, err := coreclient.FromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	application := &unikornv1core.HelmApplication{}

	if err := cli.Get(ctx, appkey, application); err != nil {
		return nil, nil, err
	}

	return application, &reference.Version, nil
}

func (a *ApplicationReferenceGetter) clusterOpenstack(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "cluster-openstack")
}

func (a *ApplicationReferenceGetter) cilium(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "cilium")
}

func (a *ApplicationReferenceGetter) openstackCloudProvider(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "openstack-cloud-provider")
}

func (a *ApplicationReferenceGetter) openstackPluginCinderCSI(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "openstack-plugin-cinder-csi")
}

func (a *ApplicationReferenceGetter) metricsServer(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "metrics-server")
}

func (a *ApplicationReferenceGetter) nvidiaGPUOperator(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "nvidia-gpu-operator")
}

func (a *ApplicationReferenceGetter) certManager(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "cert-manager")
}

func (a *ApplicationReferenceGetter) amdGPUOperator(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "amd-gpu-operator")
}

func (a *ApplicationReferenceGetter) clusterAutoscaler(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "cluster-autoscaler")
}

func (a *ApplicationReferenceGetter) clusterAutoscalerOpenstack(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "cluster-autoscaler-openstack")
}

func (a *ApplicationReferenceGetter) gatewayAPI(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "gateway-api-crds")
}

// Options allows access to CLI options in the provisioner.
type Options struct {
	// identityOptions allow the identity host and CA to be set.
	identityOptions *identityclient.Options
	// regionOptions allows the region host and CA to be set.
	regionOptions *regionclient.Options
	// clientOptions give access to client certificate information as
	// we need to talk to identity to get a token, and then to region
	// to ensure cloud identities and networks are provisioned, as well
	// as deprovisioning them.
	clientOptions coreclient.HTTPClientOptions
}

func (o *Options) AddFlags(f *pflag.FlagSet) {
	if o.identityOptions == nil {
		o.identityOptions = identityclient.NewOptions()
	}

	if o.regionOptions == nil {
		o.regionOptions = regionclient.NewOptions()
	}

	o.identityOptions.AddFlags(f)
	o.regionOptions.AddFlags(f)
	o.clientOptions.AddFlags(f)
}

// Provisioner encapsulates control plane provisioning.
type Provisioner struct {
	provisioners.Metadata

	// cluster is the Kubernetes cluster we're provisioning.
	cluster unikornv1.KubernetesCluster

	// options are documented for the type.
	options *Options
}

// New returns a new initialized provisioner object.
func New(options manager.ControllerOptions) provisioners.ManagerProvisioner {
	o, _ := options.(*Options)

	return &Provisioner{
		options: o,
	}
}

// Ensure the ManagerProvisioner interface is implemented.
var _ provisioners.ManagerProvisioner = &Provisioner{}

func (p *Provisioner) Object() unikornv1core.ManagableResourceInterface {
	return &p.cluster
}

func (p *Provisioner) identityClient(ctx context.Context) (identityapi.ClientWithResponsesInterface, error) {
	client, err := coreclient.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	issuer := identityclient.NewTokenIssuer(client, p.options.identityOptions, &p.options.clientOptions, constants.ServiceDescriptor())

	return identityclient.New(client, p.options.identityOptions, &p.options.clientOptions).ControllerClient(ctx, issuer, &p.cluster)
}

// getClusterManager gets the control plane object that owns this cluster.
func (p *Provisioner) getClusterManager(ctx context.Context) (*unikornv1.ClusterManager, error) {
	cli, err := coreclient.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	key := client.ObjectKey{
		Namespace: p.cluster.Namespace,
		Name:      p.cluster.Spec.ClusterManagerID,
	}

	var clusterManager unikornv1.ClusterManager

	if err := cli.Get(ctx, key, &clusterManager); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrClusterManager, err.Error())
	}

	return &clusterManager, nil
}

// provisionerOptions are dervied facts about a cluster used to generate the provisioner.
type provisionerOptions struct {
	// autoscaling tells whether any pools have autoscaling enabled.
	autoscaling bool
	// gpuOperatorNvidia defines whether NVIDIA GPUs are present.
	gpuVendorNvidia bool
	// gpuOperatorAMD defines whether AMD GPUs are present.
	gpuVendorAMD bool
}

func (p *Provisioner) getProvisionerOptions(options *kubernetesprovisioners.ClusterOpenstackOptions) (*provisionerOptions, error) {
	provisionerOptions := &provisionerOptions{}

	if options == nil {
		return provisionerOptions, nil
	}

	for _, pool := range p.cluster.Spec.WorkloadPools.Pools {
		if pool.Autoscaling != nil {
			provisionerOptions.autoscaling = true
		}

		callback := func(flavor regionapi.Flavor) bool {
			return flavor.Metadata.Id == pool.FlavorID
		}

		index := slices.IndexFunc(options.Flavors, callback)
		if index < 0 {
			return nil, fmt.Errorf("%w: unable to lookup flavor %s", ErrResourceDependency, pool.FlavorID)
		}

		flavor := &options.Flavors[index]

		if flavor.Spec.Gpu != nil {
			switch flavor.Spec.Gpu.Vendor {
			case regionapi.GpuVendorNVIDIA:
				provisionerOptions.gpuVendorNvidia = true
			case regionapi.GpuVendorAMD:
				provisionerOptions.gpuVendorAMD = true
			default:
				return nil, fmt.Errorf("%w: unhandled GPU vendor %v", ErrResourceDependency, flavor.Spec.Gpu.Vendor)
			}
		}
	}

	return provisionerOptions, nil
}

//nolint:cyclop
func (p *Provisioner) getProvisioner(ctx context.Context, options *kubernetesprovisioners.ClusterOpenstackOptions, provision bool) (provisioners.Provisioner, error) {
	apps := newApplicationReferenceGetter(&p.cluster)

	bundle, err := apps.getBundle(ctx)
	if err != nil {
		return nil, err
	}

	provisionerOptions, err := p.getProvisionerOptions(options)
	if err != nil {
		return nil, err
	}

	clusterManager, err := p.getClusterManager(ctx)
	if err != nil {
		return nil, err
	}

	remoteClusterManager := remotecluster.New(vcluster.NewRemoteCluster(p.cluster.Namespace, clusterManager.Name, clusterManager), false)

	clusterManagerPrefix, err := util.GetNATPrefix(ctx)
	if err != nil {
		return nil, err
	}

	remoteCluster := remotecluster.New(clusteropenstack.NewRemoteCluster(&p.cluster), true)

	clusterProvisioner := clusteropenstack.New(apps.clusterOpenstack, options, clusterManagerPrefix).InNamespace(p.cluster.Name)

	// These applications are required to get the cluster up and running, they must
	// tolerate control plane taints, be scheduled onto control plane nodes and allow
	// scale from zero.
	bootstrapProvisioner := concurrent.New("cluster bootstrap",
		cilium.New(apps.cilium),
		openstackcloudprovider.New(apps.openstackCloudProvider, options),
	)

	clusterAutoscalerProvisioner := conditional.New("cluster-autoscaler",
		func() bool { return p.cluster.AutoscalingEnabled() && provisionerOptions.autoscaling },
		concurrent.New("cluster-autoscaler",
			clusterautoscaler.New(apps.clusterAutoscaler).InNamespace(p.cluster.Name),
			clusterautoscaleropenstack.New(apps.clusterAutoscalerOpenstack).InNamespace(p.cluster.Name),
		),
	)

	addOnsApplications := []provisioners.Provisioner{
		openstackplugincindercsi.New(apps.openstackPluginCinderCSI, options),
		metricsserver.New(apps.metricsServer),
		conditional.New("nvidia-gpu-operator",
			func() bool { return p.cluster.GPUOperatorEnabled() && provisionerOptions.gpuVendorNvidia },
			nvidiagpuoperator.New(apps.nvidiaGPUOperator),
		),
		conditional.New("cert-manager",
			func() bool { return p.cluster.GPUOperatorEnabled() && provisionerOptions.gpuVendorAMD },
			certmanager.New(apps.certManager),
		),
		conditional.New("amd-gpu-operator",
			func() bool { return p.cluster.GPUOperatorEnabled() && provisionerOptions.gpuVendorAMD },
			amdgpuoperator.New(apps.amdGPUOperator),
		),
	}

	// TODO: remove this conditional when smaller versions are all retired.
	if bundle.Spec.Version.Version.Compare(semver.MustParse("v1.3.0")) >= 0 {
		addOnsApplications = append(addOnsApplications, gatewayapi.New(apps.gatewayAPI))
	}

	addonsProvisioner := concurrent.New("cluster add-ons", addOnsApplications...)

	// Create the cluster and the boostrap components in parallel, the cluster will
	// come up but never reach healthy until the CNI and cloud controller manager
	// are added.  Follow that up by the autoscaler as some addons may require worker
	// nodes to schedule onto.
	// When deprovisioning, we need to tear down the cloud controller manager before
	// the cluster vanishes as there are some required cleanup items e.g. free up
	// load balancers and floaing IPs.
	var kubernetesClusterProvisioner provisioners.Provisioner

	if provision {
		kubernetesClusterProvisioner = concurrent.New("kubernetes cluster",
			clusterProvisioner,
			remoteCluster.ProvisionOn(bootstrapProvisioner),
		)
	} else {
		kubernetesClusterProvisioner = serial.New("kubernetes cluster",
			clusterProvisioner,
			remoteCluster.ProvisionOn(bootstrapProvisioner),
		)
	}

	provisioner := remoteClusterManager.ProvisionOn(
		serial.New("kubernetes cluster",
			kubernetesClusterProvisioner,
			clusterAutoscalerProvisioner,
			remoteCluster.ProvisionOn(addonsProvisioner, remotecluster.BackgroundDeletion),
		),
	)

	return provisioner, nil
}

// managerReady gates cluster creation on the manager being up and ready.
// Due to https://github.com/argoproj/argo-cd/issues/18041 Argo will break
// quite spectacularly if you try to install an application when the requisite
// CRDs are not present yet.  As a result we need to provision the implicit
// manager serially, and that takes a long time.  So long the request times
// out, so we essentially have to defer cluster creation until we know the
// manager is working and Argo isn't going to fail.
func (p *Provisioner) managerReady(ctx context.Context) error {
	log := log.FromContext(ctx)

	clusterManager, err := p.getClusterManager(ctx)
	if err != nil {
		return err
	}

	condition, err := clusterManager.StatusConditionRead(unikornv1core.ConditionAvailable)
	if err != nil {
		return err
	}

	if condition.Reason != unikornv1core.ConditionReasonProvisioned {
		log.Info("waiting for cluster manager to become ready")

		return provisioners.ErrYield
	}

	return nil
}

// getRegionClient returns an authenticated context with a client credentials access token
// and a client.  The context must be used by subsequent API calls in order to extract
// the access token.
func (p *Provisioner) getRegionClient(ctx context.Context) (context.Context, regionapi.ClientWithResponsesInterface, error) {
	cli, err := coreclient.FromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	issuer := identityclient.NewTokenIssuer(cli, p.options.identityOptions, &p.options.clientOptions, constants.ServiceDescriptor())

	getter := regionclient.New(cli, p.options.regionOptions, &p.options.clientOptions)

	client, err := getter.ControllerClient(ctx, issuer, &p.cluster)
	if err != nil {
		return nil, nil, err
	}

	return ctx, client, nil
}

func (p *Provisioner) getFlavors(ctx context.Context, client regionapi.ClientWithResponsesInterface, organizationID, regionID string) ([]regionapi.Flavor, error) {
	resp, err := client.GetApiV1OrganizationsOrganizationIDRegionsRegionIDFlavorsWithResponse(ctx, organizationID, regionID)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, coreapiutils.ExtractError(resp.StatusCode(), resp)
	}

	flavors := *resp.JSON200

	return flavors, nil
}

func (p *Provisioner) getIdentity(ctx context.Context, client regionapi.ClientWithResponsesInterface) (*regionapi.IdentityRead, error) {
	log := log.FromContext(ctx)

	response, err := client.GetApiV1OrganizationsOrganizationIDProjectsProjectIDIdentitiesIdentityIDWithResponse(ctx, p.cluster.Labels[coreconstants.OrganizationLabel], p.cluster.Labels[coreconstants.ProjectLabel], p.cluster.Annotations[coreconstants.IdentityAnnotation])
	if err != nil {
		return nil, err
	}

	if response.StatusCode() != http.StatusOK {
		return nil, coreapiutils.ExtractError(response.StatusCode(), response)
	}

	resource := response.JSON200

	//nolint:exhaustive
	switch resource.Metadata.ProvisioningStatus {
	case coreapi.ResourceProvisioningStatusProvisioned:
		return resource, nil
	case coreapi.ResourceProvisioningStatusUnknown, coreapi.ResourceProvisioningStatusProvisioning:
		log.Info("waiting for identity to become ready")

		return nil, provisioners.ErrYield
	}

	return nil, fmt.Errorf("%w: unhandled status %s", ErrResourceDependency, resource.Metadata.ProvisioningStatus)
}

func (p *Provisioner) deleteIdentity(ctx context.Context, client regionapi.ClientWithResponsesInterface) error {
	response, err := client.DeleteApiV1OrganizationsOrganizationIDProjectsProjectIDIdentitiesIdentityIDWithResponse(ctx, p.cluster.Labels[coreconstants.OrganizationLabel], p.cluster.Labels[coreconstants.ProjectLabel], p.cluster.Annotations[coreconstants.IdentityAnnotation])
	if err != nil {
		return err
	}

	statusCode := response.StatusCode()

	// An accepted status means the API has recoded the deletion event and
	// we can delete the cluster, a not found means it's been deleted already
	// and again can proceed.  The goal here is not to leak resources.
	if statusCode != http.StatusAccepted && statusCode != http.StatusNotFound {
		return coreapiutils.ExtractError(response.StatusCode(), response)
	}

	return nil
}

func (p *Provisioner) getNetwork(ctx context.Context, client regionapi.ClientWithResponsesInterface) (*regionapi.NetworkRead, error) {
	log := log.FromContext(ctx)

	networkID, ok := p.cluster.Annotations[coreconstants.PhysicalNetworkAnnotation]
	if !ok {
		//nolint: nilnil
		return nil, nil
	}

	response, err := client.GetApiV1OrganizationsOrganizationIDProjectsProjectIDIdentitiesIdentityIDNetworksNetworkIDWithResponse(ctx, p.cluster.Labels[coreconstants.OrganizationLabel], p.cluster.Labels[coreconstants.ProjectLabel], p.cluster.Annotations[coreconstants.IdentityAnnotation], networkID)
	if err != nil {
		return nil, err
	}

	if response.StatusCode() != http.StatusOK {
		return nil, coreapiutils.ExtractError(response.StatusCode(), response)
	}

	resource := response.JSON200

	//nolint:exhaustive
	switch resource.Metadata.ProvisioningStatus {
	case coreapi.ResourceProvisioningStatusProvisioned:
		return resource, nil
	case coreapi.ResourceProvisioningStatusUnknown, coreapi.ResourceProvisioningStatusProvisioning:
		log.Info("waiting for physical network to become ready")

		return nil, provisioners.ErrYield
	}

	return nil, fmt.Errorf("%w: unhandled status %s", ErrResourceDependency, resource.Metadata.ProvisioningStatus)
}

func (p *Provisioner) getExternalNetwork(ctx context.Context, client regionapi.ClientWithResponsesInterface) (*regionapi.ExternalNetwork, error) {
	response, err := client.GetApiV1OrganizationsOrganizationIDRegionsRegionIDExternalnetworksWithResponse(ctx, p.cluster.Labels[coreconstants.OrganizationLabel], p.cluster.Spec.RegionID)
	if err != nil {
		return nil, err
	}

	if response.StatusCode() != http.StatusOK {
		return nil, coreapiutils.ExtractError(response.StatusCode(), response)
	}

	externalNetworks := *response.JSON200

	if len(externalNetworks) == 0 {
		return nil, fmt.Errorf("%w: no external networks available", ErrResourceDependency)
	}

	// NOTE: this relies on the region API enforcing ordering constraints so the selection
	// process is deterministic.
	return &externalNetworks[0], nil
}

func (p *Provisioner) identityOptions(ctx context.Context, client regionapi.ClientWithResponsesInterface) (*kubernetesprovisioners.ClusterOpenstackOptions, error) {
	identity, err := p.getIdentity(ctx, client)
	if err != nil {
		return nil, err
	}

	externalNetwork, err := p.getExternalNetwork(ctx, client)
	if err != nil {
		return nil, err
	}

	flavors, err := p.getFlavors(ctx, client, p.cluster.Labels[coreconstants.OrganizationLabel], p.cluster.Spec.RegionID)
	if err != nil {
		return nil, err
	}

	options := &kubernetesprovisioners.ClusterOpenstackOptions{
		CloudConfig:       *identity.Spec.Openstack.CloudConfig,
		Cloud:             *identity.Spec.Openstack.Cloud,
		ServerGroupID:     identity.Spec.Openstack.ServerGroupId,
		SSHKeyName:        identity.Spec.Openstack.SshKeyName,
		ExternalNetworkID: &externalNetwork.Id,
		Flavors:           flavors,
	}

	network, err := p.getNetwork(ctx, client)
	if err != nil {
		return nil, err
	}

	if network != nil {
		options.ProviderNetwork = &kubernetesprovisioners.ClusterOpenstackProviderOptions{
			NetworkID: network.Spec.Openstack.NetworkId,
			SubnetID:  network.Spec.Openstack.SubnetId,
		}
	}

	return options, nil
}

// Provision implements the Provision interface.
func (p *Provisioner) Provision(ctx context.Context) error {
	// The cluster manager is provisioned asynchronously as it takes a good minute
	// to provision, so we have to poll for readiness.
	if err := p.managerReady(ctx); err != nil {
		return err
	}

	// Likewise identity creation is provisioned asynchronously as it too takes a
	// long time, especially if a physical network is being provisioned and that
	// needs to go out and talk to swiches.
	clientContext, client, err := p.getRegionClient(ctx)
	if err != nil {
		return err
	}

	options, err := p.identityOptions(clientContext, client)
	if err != nil {
		return err
	}

	provisioner, err := p.getProvisioner(ctx, options, true)
	if err != nil {
		return err
	}

	if err := provisioner.Provision(ctx); err != nil {
		return err
	}

	return nil
}

// Deprovision implements the Provision interface.
func (p *Provisioner) Deprovision(ctx context.Context) error {
	provisioner, err := p.getProvisioner(ctx, nil, false)
	if err != nil {
		if errors.Is(err, ErrClusterManager) {
			return nil
		}

		return err
	}

	if err := provisioner.Deprovision(ctx); err != nil {
		return err
	}

	// Clean up the identity when everything has cleanly deprovisioned.
	// An accepted status means the API has recoded the deletion event and
	// we can delete the cluster, a not found means it's been deleted already
	// and again can proceed.  The goal here is not to leak resources.
	clientContext, client, err := p.getRegionClient(ctx)
	if err != nil {
		return err
	}

	if err := p.deleteIdentity(clientContext, client); err != nil {
		return err
	}

	cli, err := coreclient.FromContext(ctx)
	if err != nil {
		return err
	}

	identity, err := p.identityClient(ctx)
	if err != nil {
		return err
	}

	if err := identityclient.NewAllocations(cli, identity).Delete(ctx, &p.cluster); err != nil {
		return err
	}

	return nil
}
