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

package clustermanager

import (
	"context"
	"slices"

	"github.com/prometheus/client_golang/prometheus"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	coreclient "github.com/unikorn-cloud/core/pkg/client"
	"github.com/unikorn-cloud/core/pkg/manager"
	"github.com/unikorn-cloud/core/pkg/provisioners"
	"github.com/unikorn-cloud/core/pkg/provisioners/remotecluster"
	"github.com/unikorn-cloud/core/pkg/provisioners/serial"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/internal/applicationbundle"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/certmanager"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/clusterapi"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/vcluster"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// On home broadband it'll take about 150s to pull down images, plus any
	// readiness gates we put in the way.  If images are cached then 45s.
	//nolint:gochecknoglobals
	durationMetric = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "unikorn_clustermanager_provision_duration",
		Help: "Time taken for clustermanager to provision",
		Buckets: []float64{
			1, 5, 10, 15, 20, 30, 45, 60, 90, 120, 180, 240, 300,
		},
	})
)

//nolint:gochecknoinits
func init() {
	metrics.Registry.MustRegister(durationMetric)
}

type ApplicationReferenceGetter struct {
	clusterManager *unikornv1.ClusterManager
}

func newApplicationReferenceGetter(clusterManager *unikornv1.ClusterManager) *ApplicationReferenceGetter {
	return &ApplicationReferenceGetter{
		clusterManager: clusterManager,
	}
}

func (a *ApplicationReferenceGetter) getApplication(ctx context.Context, name string) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	namespace, err := coreclient.NamespaceFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	cli, err := coreclient.ProvisionerClientFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	appclient := applicationbundle.NewClient(cli, namespace)

	bundle, err := appclient.GetClusterManager(ctx, a.clusterManager.Spec.ApplicationBundle)
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
	return a.getApplication(ctx, "vcluster")
}

func (a *ApplicationReferenceGetter) certManager(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "cert-manager")
}

func (a *ApplicationReferenceGetter) clusterAPI(ctx context.Context) (*unikornv1core.HelmApplication, *unikornv1core.SemanticVersion, error) {
	return a.getApplication(ctx, "cluster-api")
}

// Provisioner encapsulates control plane provisioning.
type Provisioner struct {
	provisioners.Metadata

	// clusterManager is the control plane CR this deployment relates to
	clusterManager unikornv1.ClusterManager
}

// New returns a new initialized provisioner object.
func New(_ manager.ControllerOptions) provisioners.ManagerProvisioner {
	return &Provisioner{}
}

// Ensure the ManagerProvisioner interface is implemented.
var _ provisioners.ManagerProvisioner = &Provisioner{}

func (p *Provisioner) Object() unikornv1core.ManagableResourceInterface {
	return &p.clusterManager
}

// getClusterManagerProvisioner returns a provisoner that encodes control plane
// provisioning steps.
func (p *Provisioner) getClusterManagerProvisioner() provisioners.Provisioner {
	apps := newApplicationReferenceGetter(&p.clusterManager)

	remoteClusterManager := remotecluster.New(vcluster.NewRemoteCluster(p.clusterManager.Namespace, p.clusterManager.Name, &p.clusterManager), true)

	// **** sake https://github.com/argoproj/argo-cd/issues/18041
	// This should be a concurrent provision, but alas no.
	clusterAPIProvisioner := serial.New("cluster-api",
		certmanager.New(apps.certManager),
		clusterapi.New(apps.clusterAPI),
	)

	// Provision the vitual cluster, setup the remote cluster then
	// install cert manager and cluster API into it.
	return serial.New("control plane",
		vcluster.New(apps.vCluster, p.clusterManager.Name).InNamespace(p.clusterManager.Namespace),
		remoteClusterManager.ProvisionOn(clusterAPIProvisioner, remotecluster.BackgroundDeletion),
	)
}

// Provision implements the Provision interface.
func (p *Provisioner) Provision(ctx context.Context) error {
	log := log.FromContext(ctx)

	log.Info("provisioning control plane")

	timer := prometheus.NewTimer(durationMetric)
	defer timer.ObserveDuration()

	if err := p.getClusterManagerProvisioner().Provision(ctx); err != nil {
		return err
	}

	log.Info("control plane provisioned")

	return nil
}

// Deprovision implements the Provision interface.
func (p *Provisioner) Deprovision(ctx context.Context) error {
	log := log.FromContext(ctx)

	// When deleting a manager, you need to also delete any managed clusters
	// first to free up compute resources from the provider, so block until
	// this is done.
	cli, err := coreclient.ProvisionerClientFromContext(ctx)
	if err != nil {
		return err
	}

	var clusters unikornv1.KubernetesClusterList

	if err := cli.List(ctx, &clusters, &client.ListOptions{Namespace: p.clusterManager.Namespace}); err != nil {
		return err
	}

	clusters.Items = slices.DeleteFunc(clusters.Items, func(cluster unikornv1.KubernetesCluster) bool {
		return cluster.Spec.ClusterManagerID != p.clusterManager.Name
	})

	if len(clusters.Items) != 0 {
		log.Info("cluster manager has managed clusters, yielding")

		for i := range clusters.Items {
			cluster := &clusters.Items[i]

			if cluster.DeletionTimestamp != nil {
				log.Info("awaiting cluster deletion", "cluster", cluster.Name)

				continue
			}

			log.Info("triggering cluster deletion", "cluster", cluster.Name)

			if err := cli.Delete(ctx, cluster); err != nil {
				return err
			}
		}

		return provisioners.ErrYield
	}

	// Remove the control plane.
	if err := p.getClusterManagerProvisioner().Deprovision(ctx); err != nil {
		return err
	}

	return nil
}
