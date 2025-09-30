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
	"fmt"

	coreclient "github.com/unikorn-cloud/core/pkg/client"
	"github.com/unikorn-cloud/core/pkg/errors"
	coremanager "github.com/unikorn-cloud/core/pkg/manager"
	"github.com/unikorn-cloud/core/pkg/manager/options"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/constants"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/managers/clustermanager"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Factory provides methods that can build a type specific controller.
type Factory struct{}

var _ coremanager.ControllerFactory = &Factory{}

// Metadata returns the application, version and revision.
func (*Factory) Metadata() (string, string, string) {
	return constants.Application, constants.Version, constants.Revision
}

// Options returns any options to be added to the CLI flags and passed to the reconciler.
func (*Factory) Options() coremanager.ControllerOptions {
	return nil
}

// Reconciler returns a new reconciler instance.
func (*Factory) Reconciler(options *options.Options, controllerOptions coremanager.ControllerOptions, manager manager.Manager) reconcile.Reconciler {
	return coremanager.NewReconciler(options, controllerOptions, manager, clustermanager.New)
}

// RegisterWatches adds any watches that would trigger a reconcile.
func (*Factory) RegisterWatches(manager manager.Manager, controller controller.Controller) error {
	if err := controller.Watch(source.Kind(manager.GetCache(), &unikornv1.ClusterManager{}, &handler.TypedEnqueueRequestForObject[*unikornv1.ClusterManager]{}, &predicate.TypedGenerationChangedPredicate[*unikornv1.ClusterManager]{})); err != nil {
		return err
	}

	return nil
}

// Schemes allows controllers to add types to the client beyond
// the defaults defined in this repository.
func (*Factory) Schemes() []coreclient.SchemeAdder {
	return []coreclient.SchemeAdder{
		unikornv1.AddToScheme,
	}
}

func (*Factory) Upgrade(ctx context.Context, cli client.Client, options *options.Options) error {
	// v1.8.0 made cluster managers cascade delete clusters via owner references
	// rather than bespoke code.
	clusterManagers := &unikornv1.ClusterManagerList{}

	if err := cli.List(ctx, clusterManagers, &client.ListOptions{}); err != nil {
		return err
	}

	clusterManagerMap := map[string]*unikornv1.ClusterManager{}

	for i := range clusterManagers.Items {
		clusterManagerMap[clusterManagers.Items[i].Name] = &clusterManagers.Items[i]
	}

	clusters := &unikornv1.KubernetesClusterList{}

	if err := cli.List(ctx, clusters, &client.ListOptions{}); err != nil {
		return err
	}

	for i := range clusters.Items {
		cluster := &clusters.Items[i]

		clusterManager, ok := clusterManagerMap[cluster.Spec.ClusterManagerID]
		if !ok {
			return fmt.Errorf("%w: cluster %s references unknown manager", errors.ErrConsistency, cluster.Name)
		}

		ok, err := controllerutil.HasOwnerReference(cluster.OwnerReferences, clusterManager, cli.Scheme())
		if err != nil {
			return err
		}

		if ok {
			continue
		}

		if err := controllerutil.SetOwnerReference(clusterManager, cluster, cli.Scheme(), controllerutil.WithBlockOwnerDeletion(true)); err != nil {
			return err
		}

		if err := cli.Update(ctx, cluster); err != nil {
			return err
		}
	}

	return nil
}
