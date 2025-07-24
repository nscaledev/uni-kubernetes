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
	"fmt"
	"net/http"
	"slices"

	"github.com/unikorn-cloud/core/pkg/constants"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	"github.com/unikorn-cloud/core/pkg/server/conversion"
	"github.com/unikorn-cloud/core/pkg/server/errors"
	coreutil "github.com/unikorn-cloud/core/pkg/server/util"
	identitycommon "github.com/unikorn-cloud/identity/pkg/handler/common"
	identityapi "github.com/unikorn-cloud/identity/pkg/openapi"
	"github.com/unikorn-cloud/identity/pkg/principal"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/openapi"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/virtualcluster"
	provisioner "github.com/unikorn-cloud/kubernetes/pkg/provisioners/managers/virtualcluster"
	"github.com/unikorn-cloud/kubernetes/pkg/server/handler/common"
	"github.com/unikorn-cloud/kubernetes/pkg/server/handler/identity"
	"github.com/unikorn-cloud/kubernetes/pkg/server/handler/region"
	regionutil "github.com/unikorn-cloud/kubernetes/pkg/util/region"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrConsistency = goerrors.New("consistency error")

	ErrAPI = goerrors.New("remote api error")
)

// Client wraps up cluster related management handling.
type Client struct {
	// client allows Kubernetes API access.
	client client.Client

	// identity is a client to access the identity service.
	identity *identity.Client

	// region is a client to access regions.
	region *region.Client
}

// NewClient returns a new client with required parameters.
func NewClient(client client.Client, identity *identity.Client, region *region.Client) *Client {
	return &Client{
		client:   client,
		identity: identity,
		region:   region,
	}
}

// List returns all clusters owned by the implicit control plane.
func (c *Client) List(ctx context.Context, organizationID string, params openapi.GetApiV1OrganizationsOrganizationIDVirtualclustersParams) (openapi.VirtualKubernetesClusters, error) {
	result := &unikornv1.VirtualKubernetesClusterList{}

	requirement, err := labels.NewRequirement(constants.OrganizationLabel, selection.Equals, []string{organizationID})
	if err != nil {
		return nil, errors.OAuth2ServerError("failed to build label selector").WithError(err)
	}

	selector := labels.NewSelector()
	selector = selector.Add(*requirement)

	options := &client.ListOptions{
		LabelSelector: selector,
	}

	if err := c.client.List(ctx, result, options); err != nil {
		return nil, errors.OAuth2ServerError("failed to list clusters").WithError(err)
	}

	tagSelector, err := coreutil.DecodeTagSelectorParam(params.Tag)
	if err != nil {
		return nil, err
	}

	result.Items = slices.DeleteFunc(result.Items, func(resource unikornv1.VirtualKubernetesCluster) bool {
		return !resource.Spec.Tags.ContainsAll(tagSelector)
	})

	slices.SortStableFunc(result.Items, unikornv1.CompareVirtualKubernetesCluster)

	return convertList(result), nil
}

// get returns the cluster.
func (c *Client) get(ctx context.Context, namespace, clusterID string) (*unikornv1.VirtualKubernetesCluster, error) {
	result := &unikornv1.VirtualKubernetesCluster{}

	if err := c.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clusterID}, result); err != nil {
		if kerrors.IsNotFound(err) {
			return nil, errors.HTTPNotFound().WithError(err)
		}

		return nil, errors.OAuth2ServerError("unable to get cluster").WithError(err)
	}

	return result, nil
}

// regionKubernetesClient wraps up access to the remote Kubernetes cluster for
// the region.
func (c *Client) regionKubernetesClient(ctx context.Context, organizationID string, cluster *unikornv1.VirtualKubernetesCluster) (client.Client, error) {
	region, err := c.region.Get(ctx, organizationID, cluster.Spec.RegionID)
	if err != nil {
		return nil, err
	}

	kubeconfig, err := regionutil.Kubeconfig(region)
	if err != nil {
		return nil, err
	}

	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
	if err != nil {
		return nil, err
	}

	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return nil, err
	}

	getter := func() (*clientcmdapi.Config, error) {
		return &rawConfig, nil
	}

	restConfig, err := clientcmd.BuildConfigFromKubeconfigGetter("", getter)
	if err != nil {
		return nil, err
	}

	return client.New(restConfig, client.Options{})
}

// GetKubeconfig returns the kubernetes configuation associated with a cluster.
func (c *Client) GetKubeconfig(ctx context.Context, organizationID, projectID, clusterID string) ([]byte, error) {
	project, err := common.New(c.client).ProjectNamespace(ctx, organizationID, projectID)
	if err != nil {
		return nil, err
	}

	cluster, err := c.get(ctx, project.Name, clusterID)
	if err != nil {
		return nil, err
	}

	cli, err := c.regionKubernetesClient(ctx, organizationID, cluster)
	if err != nil {
		return nil, err
	}

	objectKey := client.ObjectKey{
		Namespace: provisioner.RemoteNamespace(cluster),
		Name:      "vc-" + virtualcluster.ReleaseName(cluster),
	}

	secret := &corev1.Secret{}

	if err := cli.Get(ctx, objectKey, secret); err != nil {
		if kerrors.IsNotFound(err) {
			return nil, errors.HTTPNotFound().WithError(err)
		}

		return nil, errors.OAuth2ServerError("unable to get cluster configuration").WithError(err)
	}

	return secret.Data["config"], nil
}

func (c *Client) generateAllocations(ctx context.Context, organizationID string, resource *unikornv1.VirtualKubernetesCluster) (*identityapi.AllocationWrite, error) {
	flavors, err := c.region.Flavors(ctx, organizationID, resource.Spec.RegionID)
	if err != nil {
		return nil, err
	}

	var serversCommitted int

	var gpusCommitted int

	for _, pool := range resource.Spec.WorkloadPools {
		serversCommitted += pool.Replicas

		flavorByID := func(f regionapi.Flavor) bool {
			return f.Metadata.Id == pool.FlavorID
		}

		index := slices.IndexFunc(flavors, flavorByID)
		if index < 0 {
			return nil, fmt.Errorf("%w: flavorID does not exist", ErrConsistency)
		}

		flavor := flavors[index]

		if flavor.Spec.Gpu != nil {
			gpusCommitted += serversCommitted * flavor.Spec.Gpu.PhysicalCount
		}
	}

	request := &identityapi.AllocationWrite{
		Metadata: coreapi.ResourceWriteMetadata{
			Name: constants.UndefinedName,
		},
		Spec: identityapi.AllocationSpec{
			Kind: "virtualkubernetescluster",
			Id:   resource.Name,
			Allocations: identityapi.ResourceAllocationList{
				{
					Kind:      "clusters",
					Committed: 1,
					Reserved:  0,
				},
				{
					Kind:      "servers",
					Committed: serversCommitted,
					Reserved:  0,
				},
				{
					Kind:      "gpus",
					Committed: gpusCommitted,
					Reserved:  0,
				},
			},
		},
	}

	return request, nil
}

func (c *Client) createAllocation(ctx context.Context, resource *unikornv1.VirtualKubernetesCluster) (*identityapi.AllocationRead, error) {
	principal, err := principal.GetPrincipal(ctx)
	if err != nil {
		return nil, err
	}

	allocations, err := c.generateAllocations(ctx, principal.OrganizationID, resource)
	if err != nil {
		return nil, err
	}

	client, err := c.identity.Client(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := client.PostApiV1OrganizationsOrganizationIDProjectsProjectIDAllocationsWithResponse(ctx, principal.OrganizationID, principal.ProjectID, *allocations)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusCreated {
		return nil, fmt.Errorf("%w: unexpected status code %d", ErrAPI, resp.StatusCode())
	}

	return resp.JSON201, nil
}

func (c *Client) updateAllocation(ctx context.Context, resource *unikornv1.VirtualKubernetesCluster) error {
	principal, err := principal.FromResource(resource)
	if err != nil {
		return err
	}

	allocations, err := c.generateAllocations(ctx, principal.OrganizationID, resource)
	if err != nil {
		return err
	}

	client, err := c.identity.Client(ctx)
	if err != nil {
		return err
	}

	resp, err := client.PutApiV1OrganizationsOrganizationIDProjectsProjectIDAllocationsAllocationIDWithResponse(ctx, principal.OrganizationID, principal.ProjectID, resource.Annotations[constants.AllocationAnnotation], *allocations)
	if err != nil {
		return err
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("%w: unexpected status code %d", ErrAPI, resp.StatusCode())
	}

	return nil
}

func (c *Client) deleteAllocation(ctx context.Context, resource *unikornv1.VirtualKubernetesCluster) error {
	principal, err := principal.FromResource(resource)
	if err != nil {
		return err
	}

	client, err := c.identity.Client(ctx)
	if err != nil {
		return err
	}

	resp, err := client.DeleteApiV1OrganizationsOrganizationIDProjectsProjectIDAllocationsAllocationIDWithResponse(ctx, principal.OrganizationID, principal.ProjectID, resource.Annotations[constants.AllocationAnnotation])
	if err != nil {
		return err
	}

	if resp.StatusCode() != http.StatusAccepted {
		return fmt.Errorf("%w: unexpected status code %d", ErrAPI, resp.StatusCode())
	}

	return nil
}

func (c *Client) applyCloudSpecificConfiguration(allocation *identityapi.AllocationRead, cluster *unikornv1.VirtualKubernetesCluster) {
	// Save the identity ID for later cleanup.
	if cluster.Annotations == nil {
		cluster.Annotations = map[string]string{}
	}

	cluster.Annotations[constants.AllocationAnnotation] = allocation.Metadata.Id
}

func metadataMutator(required, current metav1.Object) error {
	req := required.GetAnnotations()
	if req == nil {
		req = map[string]string{}
	}

	cur := current.GetAnnotations()

	// Preserve the allocation.
	// NOTE: these are guarded by a validating admission policy so should exist.
	if v, ok := cur[constants.AllocationAnnotation]; ok {
		req[constants.AllocationAnnotation] = v
	}

	required.SetAnnotations(req)

	return nil
}

type appBundleLister interface {
	ListVirtualCluster(ctx context.Context) (*unikornv1.VirtualKubernetesClusterApplicationBundleList, error)
}

// Create creates the implicit cluster identified by the JTW claims.
func (c *Client) Create(ctx context.Context, appclient appBundleLister, organizationID, projectID string, request *openapi.VirtualKubernetesClusterWrite) (*openapi.VirtualKubernetesClusterRead, error) {
	namespace, err := common.New(c.client).ProjectNamespace(ctx, organizationID, projectID)
	if err != nil {
		return nil, err
	}

	cluster, err := newGenerator(c.client, namespace.Name, organizationID, projectID).generate(ctx, appclient, request)
	if err != nil {
		return nil, err
	}

	allocation, err := c.createAllocation(ctx, cluster)
	if err != nil {
		return nil, errors.OAuth2ServerError("failed to create quota allocation").WithError(err)
	}

	c.applyCloudSpecificConfiguration(allocation, cluster)

	if err := c.client.Create(ctx, cluster); err != nil {
		return nil, errors.OAuth2ServerError("failed to create cluster").WithError(err)
	}

	return convert(cluster), nil
}

// Delete deletes the implicit cluster identified by the JTW claims.
func (c *Client) Delete(ctx context.Context, organizationID, projectID, clusterID string) error {
	namespace, err := common.New(c.client).ProjectNamespace(ctx, organizationID, projectID)
	if err != nil {
		return err
	}

	cluster, err := c.get(ctx, namespace.Name, clusterID)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return errors.HTTPNotFound().WithError(err)
		}

		return errors.OAuth2ServerError("failed to get cluster")
	}

	if err := c.client.Delete(ctx, cluster); err != nil {
		if kerrors.IsNotFound(err) {
			return errors.HTTPNotFound().WithError(err)
		}

		return errors.OAuth2ServerError("failed to delete cluster").WithError(err)
	}

	if err := c.deleteAllocation(ctx, cluster); err != nil {
		return errors.OAuth2ServerError("failed to delete quota allocation").WithError(err)
	}

	return nil
}

// Update implements read/modify/write for the cluster.
func (c *Client) Update(ctx context.Context, appclient appBundleLister, organizationID, projectID, clusterID string, request *openapi.VirtualKubernetesClusterWrite) error {
	namespace, err := common.New(c.client).ProjectNamespace(ctx, organizationID, projectID)
	if err != nil {
		return err
	}

	if namespace.DeletionTimestamp != nil {
		return errors.OAuth2InvalidRequest("control plane is being deleted")
	}

	current, err := c.get(ctx, namespace.Name, clusterID)
	if err != nil {
		return err
	}

	required, err := newGenerator(c.client, namespace.Name, organizationID, projectID).withExisting(current).generate(ctx, appclient, request)
	if err != nil {
		return err
	}

	if err := conversion.UpdateObjectMetadata(required, current, identitycommon.IdentityMetadataMutator, metadataMutator); err != nil {
		return errors.OAuth2ServerError("failed to merge metadata").WithError(err)
	}

	// Experience has taught me that modifying caches by accident is a bad thing
	// so be extra safe and deep copy the existing resource.
	updated := current.DeepCopy()
	updated.Labels = required.Labels
	updated.Annotations = required.Annotations
	updated.Spec = required.Spec

	if err := c.updateAllocation(ctx, updated); err != nil {
		return errors.OAuth2ServerError("failed to update quota allocation").WithError(err)
	}

	if err := c.client.Patch(ctx, updated, client.MergeFrom(current)); err != nil {
		return errors.OAuth2ServerError("failed to patch cluster").WithError(err)
	}

	return nil
}
