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
	coreclient "github.com/unikorn-cloud/core/pkg/client"
	coremanager "github.com/unikorn-cloud/core/pkg/manager"
	"github.com/unikorn-cloud/core/pkg/manager/options"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/constants"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/managers/cluster"

	"sigs.k8s.io/controller-runtime/pkg/controller"
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
	return &cluster.Options{}
}

// Reconciler returns a new reconciler instance.
func (*Factory) Reconciler(options *options.Options, controlerOptions coremanager.ControllerOptions, manager manager.Manager) reconcile.Reconciler {
	return coremanager.NewReconciler(options, controlerOptions, manager, cluster.New)
}

// RegisterWatches adds any watches that would trigger a reconcile.
func (*Factory) RegisterWatches(manager manager.Manager, controller controller.Controller) error {
	// Any changes to the cluster spec, trigger a reconcile.
	if err := controller.Watch(source.Kind(manager.GetCache(), &unikornv1.KubernetesCluster{}, &handler.TypedEnqueueRequestForObject[*unikornv1.KubernetesCluster]{}, &predicate.TypedGenerationChangedPredicate[*unikornv1.KubernetesCluster]{})); err != nil {
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
