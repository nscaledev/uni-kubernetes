/*
Copyright 2025 the Unikorn Authors.

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

package virtualcluster

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/pflag"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/core/pkg/cd"
	coreclient "github.com/unikorn-cloud/core/pkg/client"
	coreconstants "github.com/unikorn-cloud/core/pkg/constants"
	"github.com/unikorn-cloud/core/pkg/manager"
	"github.com/unikorn-cloud/core/pkg/provisioners"
	"github.com/unikorn-cloud/core/pkg/provisioners/remotecluster"
	identityclient "github.com/unikorn-cloud/identity/pkg/client"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/internal/applicationbundle"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/virtualcluster"
	regionutil "github.com/unikorn-cloud/kubernetes/pkg/util/region"
	regionclient "github.com/unikorn-cloud/region/pkg/client"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrLabel = errors.New("required label missing")

	ErrAnnotation = errors.New("required annotation missing")

	ErrResourceDependency = errors.New("resource dependency error")
)

type ApplicationReferenceGetter struct {
	cluster *unikornv1.VirtualKubernetesCluster
}

func newApplicationReferenceGetter(cluster *unikornv1.VirtualKubernetesCluster) *ApplicationReferenceGetter {
	return &ApplicationReferenceGetter{
		cluster: cluster,
	}
}

func (a *ApplicationReferenceGetter) getApplication(ctx context.Context, name string) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	namespace, err := coreclient.NamespaceFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	// TODO: we could cache this, it's from a cache anyway, so quite cheap...
	cli, err := coreclient.FromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	appclient := applicationbundle.NewClient(cli, namespace)

	bundle, err := appclient.GetVirtualKubernetesCluster(ctx, a.cluster.Spec.ApplicationBundle)
	if err != nil {
		return nil, nil, err
	}

	reference, err := bundle.Spec.GetApplication(name)
	if err != nil {
		return nil, nil, err
	}

	appkey := client.ObjectKey{
		Namespace: namespace,
		Name:      *reference.Name,
	}

	application := &unikornv1core.HelmApplication{}

	if err := cli.Get(ctx, appkey, application); err != nil {
		return nil, nil, err
	}

	return application, &reference.Version, nil
}

func (a *ApplicationReferenceGetter) vCluster(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "virtualcluster")
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
	clientOptions      coreclient.HTTPClientOptions
	provisionerOptions virtualcluster.ProvisionerOptions
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
	o.provisionerOptions.AddFlags(f)
}

// Provisioner encapsulates control plane provisioning.
type Provisioner struct {
	provisioners.Metadata

	// cluster is the Kubernetes cluster we're provisioning.
	cluster unikornv1.VirtualKubernetesCluster

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

// getRegionClient returns an authenticated context with a client credentials access token
// and a client.  The context must be used by subsequent API calls in order to extract
// the access token.
func (p *Provisioner) getRegionClient(ctx context.Context, traceName string) (context.Context, regionapi.ClientWithResponsesInterface, error) {
	cli, err := coreclient.FromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	return regionutil.ControllerClient(ctx, cli, &p.options.clientOptions, p.options.identityOptions, p.options.regionOptions, traceName, &p.cluster)
}

// getKubeconfig reads the kubeconfig from the Kubernetes region.
func (p *Provisioner) getKubeconfig(ctx context.Context, client regionapi.ClientWithResponsesInterface) ([]byte, error) {
	organizationID, ok := p.cluster.Labels[coreconstants.OrganizationLabel]
	if !ok {
		return nil, fmt.Errorf("%w: cluster missing organization ID", ErrLabel)
	}

	region, err := regionutil.Region(ctx, client, organizationID, p.cluster.Spec.RegionID)
	if err != nil {
		return nil, err
	}

	return regionutil.Kubeconfig(region)
}

type regionRemote struct {
	cluster    *unikornv1.VirtualKubernetesCluster
	kubeconfig []byte
}

func (r *regionRemote) ID() *cd.ResourceIdentifier {
	resourceLabels, _ := r.cluster.ResourceLabels()

	var labels []cd.ResourceIdentifierLabel

	for _, label := range coreconstants.LabelPriorities() {
		if value, ok := resourceLabels[label]; ok {
			labels = append(labels, cd.ResourceIdentifierLabel{
				Name:  label,
				Value: value,
			})
		}
	}

	return &cd.ResourceIdentifier{
		Name:   "virtualcluster",
		Labels: labels,
	}
}

func (r *regionRemote) Config(ctx context.Context) (*clientcmdapi.Config, error) {
	config, err := clientcmd.NewClientConfigFromBytes(r.kubeconfig)
	if err != nil {
		return nil, err
	}

	rawConfig, err := config.RawConfig()
	if err != nil {
		return nil, err
	}

	return &rawConfig, nil
}

func RemoteNamespace(cluster *unikornv1.VirtualKubernetesCluster) string {
	return "virtualcluster-" + cluster.Name
}

func (p *Provisioner) getProvisioner(kubeconfig []byte) provisioners.Provisioner {
	apps := newApplicationReferenceGetter(&p.cluster)

	regionGenerator := &regionRemote{
		cluster:    &p.cluster,
		kubeconfig: kubeconfig,
	}

	remoteCluster := remotecluster.New(regionGenerator, true)

	// TODO: we need some machines to run the cluster on, separate
	// from the workload pool.  This information and the scheduling
	// stuff needs passing into the provisioner.
	provisioner := remoteCluster.ProvisionOn(
		// The namespace gets a prefix so it's easier to distinguish for automation and eyeballs.
		virtualcluster.New(apps.vCluster, p.options.provisionerOptions).InNamespace(RemoteNamespace(&p.cluster)),
		// The remote cluster is static in this case, and is shared by multiple different
		// virtual clusters.  Each virtual cluster will assume management of the secret so
		// the name MUST be unique across the whole platform, otherwise deleting one vcluster
		// will result in the CD layer losing contact with the all the others.  If you are
		// deleting multiple at the same time, this will result in resource leaks.
		remotecluster.WithPrefix("virtualcluster-"+p.cluster.Name),
	)

	return provisioner
}

// Provision implements the Provision interface.
func (p *Provisioner) Provision(ctx context.Context) error {
	clientContext, client, err := p.getRegionClient(ctx, "provision")
	if err != nil {
		return err
	}

	kubeconfig, err := p.getKubeconfig(clientContext, client)
	if err != nil {
		return err
	}

	// TODO: we need to schedule nodes based on the cluster pool requirements and
	// then have the virtual cluster tolerate whatever taint we apply to worked nodes
	// and then have all pods acquire a label selector during sync to the host.
	if err := p.getProvisioner(kubeconfig).Provision(ctx); err != nil {
		return err
	}

	return nil
}

// Deprovision implements the Provision interface.
func (p *Provisioner) Deprovision(ctx context.Context) error {
	clientContext, client, err := p.getRegionClient(ctx, "provision")
	if err != nil {
		return err
	}

	kubeconfig, err := p.getKubeconfig(clientContext, client)
	if err != nil {
		return err
	}

	if err := p.getProvisioner(kubeconfig).Deprovision(ctx); err != nil {
		return err
	}

	// TODO: we need to free any allocated nodes here.
	return nil
}
