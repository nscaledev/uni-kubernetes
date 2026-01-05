/*
Copyright 2022-2024 EscherCloud.
Copyright 2024-2025 the Unikorn Authors.
Copyright 2026 Nscale.

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
	"errors"
	"fmt"

	"github.com/unikorn-cloud/core/pkg/cd"
	coreclient "github.com/unikorn-cloud/core/pkg/client"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	// ErrWorkloadPoolMissing is returned when we expect to find a machine deployment
	// for a workload pool, but can't.  This is an indication that Argo hasn't yet
	// sychronized the resources correctly.  We risk a race otherwise where it gets
	// created when we check for existence, it gets created, and we immediately delete it.
	ErrWorkloadPoolMissing = errors.New("unable to locate expected workload pool")
)

// filterOwnedResources removes any resources that aren't owned by the cluster.
func (p *Provisioner) filterOwnedResources(cluster *unikornv1.KubernetesCluster, resources []unstructured.Unstructured) []unstructured.Unstructured {
	var filtered []unstructured.Unstructured

	for _, resource := range resources {
		ownerReferences := resource.GetOwnerReferences()

		for _, ownerReference := range ownerReferences {
			if ownerReference.Kind != "Cluster" || ownerReference.Name != releaseName(cluster) {
				continue
			}

			filtered = append(filtered, resource)
		}
	}

	return filtered
}

// getOwnedResource returns resources of the specified API version/kind that belong
// to the cluster.
func (p *Provisioner) getOwnedResource(ctx context.Context, c client.Client, apiVersion, kind string) ([]unstructured.Unstructured, error) {
	//nolint:forcetypeassert
	cluster := application.FromContext(ctx).(*unikornv1.KubernetesCluster)

	objects := &unstructured.UnstructuredList{
		Object: map[string]any{
			"apiVersion": apiVersion,
			"kind":       kind,
		},
	}

	options := &client.ListOptions{
		Namespace: cluster.Name,
	}

	if err := c.List(ctx, objects, options); err != nil {
		return nil, err
	}

	return p.filterOwnedResources(cluster, objects.Items), nil
}

// getMachineDeployments gets all live machine deployments for the cluster.
func (p *Provisioner) getMachineDeployments(ctx context.Context, c client.Client) ([]unstructured.Unstructured, error) {
	// TODO: this is flaky as hell, due to hard coded versions, needs a fix upstream.
	return p.getOwnedResource(ctx, c, "cluster.x-k8s.io/v1beta1", "MachineDeployment")
}

// getKubeadmConfigTemplates gets all live config templates for the cluster.
func (p *Provisioner) getKubeadmConfigTemplates(ctx context.Context, c client.Client) ([]unstructured.Unstructured, error) {
	// TODO: this is flaky as hell, due to hard coded versions, needs a fix upstream.
	return p.getOwnedResource(ctx, c, "bootstrap.cluster.x-k8s.io/v1beta1", "KubeadmConfigTemplate")
}

// getKubeadmControlPlanes gets all live control planes for the cluster.
func (p *Provisioner) getKubeadmControlPlanes(ctx context.Context, c client.Client) ([]unstructured.Unstructured, error) {
	// TODO: this is flaky as hell, due to hard coded versions, needs a fix upstream.
	return p.getOwnedResource(ctx, c, "controlplane.cluster.x-k8s.io/v1beta1", "KubeadmControlPlane")
}

// getOpenstackMachineTemplates gets all live machine templates for the cluster.
func (p *Provisioner) getOpenstackMachineTemplates(ctx context.Context, c client.Client) ([]unstructured.Unstructured, error) {
	// TODO: this is flaky as hell, due to hard coded versions, needs a fix upstream.
	return p.getOwnedResource(ctx, c, "infrastructure.cluster.x-k8s.io/v1beta1", "OpenStackMachineTemplate")
}

// resourceExistsUnstructured tells whether the resource exists in the
// expected list of names.
func resourceExistsUnstructured(o unstructured.Unstructured, names []string) bool {
	for _, name := range names {
		if name == o.GetName() {
			return true
		}
	}

	return false
}

// machineDeploymentForWorkloadPool finds the resource that belongs to the named
// workload pool.
func machineDeploymentForWorkloadPool(objects []unstructured.Unstructured, name string) (*unstructured.Unstructured, error) {
	for i, object := range objects {
		if value, ok := object.GetAnnotations()["pool.unikorn-cloud.org/name"]; ok {
			if value == name {
				return &objects[i], nil
			}
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrWorkloadPoolMissing, name)
}

// getExpectedMachineDeployments finds the expected machine deployments based on the
// workload pool name annotations.
func (p *Provisioner) getExpectedMachineDeployments(cluster *unikornv1.KubernetesCluster, objects []unstructured.Unstructured) ([]unstructured.Unstructured, error) {
	filtered := make([]unstructured.Unstructured, len(cluster.Spec.WorkloadPools.Pools))

	for i, pool := range cluster.Spec.WorkloadPools.Pools {
		object, err := machineDeploymentForWorkloadPool(objects, pool.Name)
		if err != nil {
			return nil, err
		}

		filtered[i] = *object
	}

	return filtered, nil
}

// getExpectedKubeadmConfigTemplateNames extracts the expected config templates from the
// deployment references.
func getExpectedKubeadmConfigTemplateNames(deployments []unstructured.Unstructured) []string {
	names := make([]string, len(deployments))

	for i, deployment := range deployments {
		// TODO: may be not ok.
		names[i], _, _ = unstructured.NestedString(deployment.Object, "spec", "template", "spec", "bootstrap", "configRef", "name")
	}

	return names
}

// getExpectedOpenstackMachineTemplateNames extracts the expected machine templates from the
// deployment references.
func getExpectedOpenstackMachineTemplateNames(deployments []unstructured.Unstructured, controlPlanes []unstructured.Unstructured) []string {
	//nolint: prealloc
	var names []string

	for _, deployment := range deployments {
		// TODO: may be not ok.
		name, _, _ := unstructured.NestedString(deployment.Object, "spec", "template", "spec", "infrastructureRef", "name")

		names = append(names, name)
	}

	for _, controlPlane := range controlPlanes {
		name, _, _ := unstructured.NestedString(controlPlane.Object, "spec", "machineTemplate", "infrastructureRef", "name")

		names = append(names, name)
	}

	return names
}

// deleteForeignResources removes any resources in the given object set that
// don't have a corresponding name in the allowed list.
func deleteForeignResources(ctx context.Context, c client.Client, objects []unstructured.Unstructured, allowed []string) error {
	log := log.FromContext(ctx)

	for i, o := range objects {
		if resourceExistsUnstructured(o, allowed) {
			continue
		}

		log.Info("deleting orphaned resource", "kind", o.GetKind(), "name", o.GetName())

		if err := c.Delete(ctx, &objects[i]); err != nil {
			return err
		}
	}

	return nil
}

// deleteOrphanedMachineDeployments does just that. So what happens when you
// delete a workload pool is that the application notes it's no longer in the
// manifest, BUT, and I like big buts, cluster-api has added an owner reference,
// so Argo thinks it's an implicitly created resource now.  So, what we need to
// do is manually delete any orphaned MachineDeployments.
//
//nolint:cyclop
func (p *Provisioner) deleteOrphanedMachineDeployments(ctx context.Context) error {
	if cd.FromContext(ctx).Kind() != cd.DriverKindArgoCD {
		return nil
	}

	//nolint:forcetypeassert
	cluster := application.FromContext(ctx).(*unikornv1.KubernetesCluster)

	clusterContext, err := coreclient.ClusterFromContext(ctx)
	if err != nil {
		return err
	}

	client := clusterContext.Client

	deployments, err := p.getMachineDeployments(ctx, client)
	if err != nil {
		return err
	}

	kubeadmConfigTemplates, err := p.getKubeadmConfigTemplates(ctx, client)
	if err != nil {
		return err
	}

	kubeadmControlPlanes, err := p.getKubeadmControlPlanes(ctx, client)
	if err != nil {
		return err
	}

	openstackMachineTemplates, err := p.getOpenstackMachineTemplates(ctx, client)
	if err != nil {
		return err
	}

	expectedDeployments, err := p.getExpectedMachineDeployments(cluster, deployments)
	if err != nil {
		return err
	}

	deploymentNames := make([]string, len(expectedDeployments))

	for i, md := range expectedDeployments {
		deploymentNames[i] = md.GetName()
	}

	// Get the expected kubeadm config template and openstack machine template names from
	// the deployments.  These are generated by Helm, and unguessable.
	kubeadmConfigTemplatesNames := getExpectedKubeadmConfigTemplateNames(expectedDeployments)
	openstackMachineTemplatesNames := getExpectedOpenstackMachineTemplateNames(expectedDeployments, kubeadmControlPlanes)

	if err := deleteForeignResources(ctx, client, deployments, deploymentNames); err != nil {
		return err
	}

	if err := deleteForeignResources(ctx, client, kubeadmConfigTemplates, kubeadmConfigTemplatesNames); err != nil {
		return err
	}

	if err := deleteForeignResources(ctx, client, openstackMachineTemplates, openstackMachineTemplatesNames); err != nil {
		return err
	}

	return nil
}
