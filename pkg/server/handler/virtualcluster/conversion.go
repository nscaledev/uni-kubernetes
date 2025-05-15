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
	goerrors "errors"
	"slices"

	"github.com/unikorn-cloud/core/pkg/server/conversion"
	"github.com/unikorn-cloud/core/pkg/server/errors"
	"github.com/unikorn-cloud/identity/pkg/middleware/authorization"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/internal/applicationbundle"
	"github.com/unikorn-cloud/kubernetes/pkg/openapi"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrResourceLookup is raised when we are looking for a referenced resource
	// but cannot find it.
	ErrResourceLookup = goerrors.New("could not find the requested resource")

	// ErrUnhandledCase is raised when an unhandled switch case is encountered.
	ErrUnhandledCase = goerrors.New("handled case")
)

// generator wraps up the myriad things we need to pass around as an object
// rather than a whole bunch of arguments.
type generator struct {
	// client allows Kubernetes access.
	client client.Client
	// namespace the resource is provisioned in.
	namespace string
	// organizationID is the unique organization identifier.
	organizationID string
	// projectID is the unique project identifier.
	projectID string
	// existing is the existing cluster used to preserve options
	// across updates.  This does two things, ensures we don't accidentally
	// pick up new defaults, and we preserve any modifications that were
	// made in a support capacity.
	existing *unikornv1.VirtualKubernetesCluster
}

func newGenerator(client client.Client, namespace, organizationID, projectID string) *generator {
	return &generator{
		client:         client,
		namespace:      namespace,
		organizationID: organizationID,
		projectID:      projectID,
	}
}

func (g *generator) withExisting(existing *unikornv1.VirtualKubernetesCluster) *generator {
	g.existing = existing

	return g
}

// convertWorkloadPool converts from a custom resource into the API definition.
func convertWorkloadPool(in *unikornv1.VirtualKubernetesClusterWorkloadPoolSpec) openapi.VirtualKubernetesClusterWorkloadPool {
	workloadPool := openapi.VirtualKubernetesClusterWorkloadPool{
		Name:     in.Name,
		Replicas: in.Replicas,
		FlavorId: in.FlavorID,
	}

	return workloadPool
}

// convertWorkloadPools converts from a custom resource into the API definition.
func convertWorkloadPools(in *unikornv1.VirtualKubernetesCluster) []openapi.VirtualKubernetesClusterWorkloadPool {
	workloadPools := make([]openapi.VirtualKubernetesClusterWorkloadPool, len(in.Spec.WorkloadPools))

	for i := range in.Spec.WorkloadPools {
		workloadPools[i] = convertWorkloadPool(&in.Spec.WorkloadPools[i])
	}

	return workloadPools
}

// convert converts from a custom resource into the API definition.
func convert(in *unikornv1.VirtualKubernetesCluster) *openapi.VirtualKubernetesClusterRead {
	out := &openapi.VirtualKubernetesClusterRead{
		Metadata: conversion.ProjectScopedResourceReadMetadata(in, in.Spec.Tags),
		Spec: openapi.VirtualKubernetesClusterSpec{
			RegionId:      in.Spec.RegionID,
			WorkloadPools: convertWorkloadPools(in),
		},
	}

	return out
}

// uconvertList converts from a custom resource list into the API definition.
func convertList(in *unikornv1.VirtualKubernetesClusterList) openapi.VirtualKubernetesClusters {
	out := make(openapi.VirtualKubernetesClusters, len(in.Items))

	for i := range in.Items {
		out[i] = *convert(&in.Items[i])
	}

	return out
}

// defaultApplicationBundle returns a default application bundle.
func (g *generator) defaultApplicationBundle(ctx context.Context) (*unikornv1.VirtualKubernetesClusterApplicationBundle, error) {
	applicationBundles, err := applicationbundle.NewClient(g.client).ListVirtualCluster(ctx)
	if err != nil {
		return nil, errors.OAuth2ServerError("failed to list application bundles").WithError(err)
	}

	applicationBundles.Items = slices.DeleteFunc(applicationBundles.Items, func(bundle unikornv1.VirtualKubernetesClusterApplicationBundle) bool {
		if bundle.Spec.Preview {
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

	// Return the newest bundle
	return &applicationBundles.Items[len(applicationBundles.Items)-1], nil
}

// generateWorkloadPools generates the workload pools part of a cluster.
func generateWorkloadPools(request *openapi.VirtualKubernetesClusterWrite) []unikornv1.VirtualKubernetesClusterWorkloadPoolSpec {
	workloadPools := make([]unikornv1.VirtualKubernetesClusterWorkloadPoolSpec, len(request.Spec.WorkloadPools))

	for i := range request.Spec.WorkloadPools {
		pool := &request.Spec.WorkloadPools[i]

		workloadPool := unikornv1.VirtualKubernetesClusterWorkloadPoolSpec{
			Name:     pool.Name,
			Replicas: pool.Replicas,
			FlavorID: pool.FlavorId,
		}

		workloadPools[i] = workloadPool
	}

	return workloadPools
}

// preserveDefaulted recognizes that, while we try to be opinionated and do things for
// the end user, there are operation reasons for disabling things, and preventing surprise
// upgrades when you update a cluster.
func (g *generator) preserveDefaultedFields(cluster *unikornv1.VirtualKubernetesCluster) {
	if g.existing == nil {
		return
	}

	cluster.Spec.ApplicationBundle = g.existing.Spec.ApplicationBundle
	cluster.Spec.ApplicationBundleAutoUpgrade = g.existing.Spec.ApplicationBundleAutoUpgrade
}

// generate generates the full cluster custom resource.
// TODO: there are a lot of parameters being passed about, we should make this
// a struct and pass them as a single blob.
func (g *generator) generate(ctx context.Context, request *openapi.VirtualKubernetesClusterWrite) (*unikornv1.VirtualKubernetesCluster, error) {
	applicationBundle, err := g.defaultApplicationBundle(ctx)
	if err != nil {
		return nil, err
	}

	info, err := authorization.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	cluster := &unikornv1.VirtualKubernetesCluster{
		ObjectMeta: conversion.NewObjectMetadata(&request.Metadata, g.namespace, info.Userinfo.Sub).WithOrganization(g.organizationID).WithProject(g.projectID).Get(),
		Spec: unikornv1.VirtualKubernetesClusterSpec{
			Tags:                         conversion.GenerateTagList(request.Metadata.Tags),
			RegionID:                     request.Spec.RegionId,
			ApplicationBundle:            applicationBundle.Name,
			ApplicationBundleAutoUpgrade: &unikornv1.ApplicationBundleAutoUpgradeSpec{},
			WorkloadPools:                generateWorkloadPools(request),
		},
	}

	g.preserveDefaultedFields(cluster)

	return cluster, nil
}
