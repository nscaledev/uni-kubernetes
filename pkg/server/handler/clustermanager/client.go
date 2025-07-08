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

	"github.com/unikorn-cloud/core/pkg/constants"
	coreopenapi "github.com/unikorn-cloud/core/pkg/openapi"
	"github.com/unikorn-cloud/core/pkg/server/conversion"
	"github.com/unikorn-cloud/core/pkg/server/errors"
	identitycommon "github.com/unikorn-cloud/identity/pkg/handler/common"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/openapi"
	"github.com/unikorn-cloud/kubernetes/pkg/server/handler/common"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Client wraps up control plane related management handling.
type Client struct {
	// client allows Kubernetes API access.
	client client.Client
}

// NewClient returns a new client with required parameters.
func NewClient(client client.Client) *Client {
	return &Client{
		client: client,
	}
}

type appBundleLister interface {
	ListClusterManager(ctx context.Context) (*unikornv1.ClusterManagerApplicationBundleList, error)
}

// CreateImplicit is called when a cluster creation call is made and a control plane is not specified.
func (c *Client) CreateImplicit(ctx context.Context, appclient appBundleLister, organizationID, projectID string) (*unikornv1.ClusterManager, error) {
	log := log.FromContext(ctx)

	namespace, err := common.New(c.client).ProjectNamespace(ctx, organizationID, projectID)
	if err != nil {
		return nil, err
	}

	var existing unikornv1.ClusterManagerList

	options := &client.ListOptions{
		Namespace:     namespace.Name,
		LabelSelector: labels.SelectorFromSet(labels.Set{constants.NameLabel: "default"}),
	}

	if err := c.client.List(ctx, &existing, options); err != nil {
		return nil, err
	}

	if len(existing.Items) != 0 {
		return &existing.Items[0], nil
	}

	log.Info("creating implicit control plane")

	request := &openapi.ClusterManagerWrite{
		Metadata: coreopenapi.ResourceWriteMetadata{
			Name:        "default",
			Description: ptr.To("Implicitly provisioned cluster controller"),
		},
	}

	resource, err := c.create(ctx, appclient, organizationID, projectID, request)
	if err != nil {
		return nil, err
	}

	return resource, nil
}

// convert converts from Kubernetes into OpenAPI types.
func (c *Client) convert(in *unikornv1.ClusterManager) *openapi.ClusterManagerRead {
	out := &openapi.ClusterManagerRead{
		Metadata: conversion.ProjectScopedResourceReadMetadata(in, in.Spec.Tags),
	}

	return out
}

// convertList converts from Kubernetes into OpenAPI types.
func (c *Client) convertList(in *unikornv1.ClusterManagerList) openapi.ClusterManagers {
	out := make(openapi.ClusterManagers, len(in.Items))

	for i := range in.Items {
		out[i] = *c.convert(&in.Items[i])
	}

	return out
}

// List returns all control planes.
func (c *Client) List(ctx context.Context, organizationID string) (openapi.ClusterManagers, error) {
	result := &unikornv1.ClusterManagerList{}

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
		return nil, errors.OAuth2ServerError("failed to list control planes").WithError(err)
	}

	slices.SortStableFunc(result.Items, unikornv1.CompareClusterManager)

	return c.convertList(result), nil
}

// get returns the control plane.
func (c *Client) get(ctx context.Context, namespace, clusterManagerID string) (*unikornv1.ClusterManager, error) {
	result := &unikornv1.ClusterManager{}

	if err := c.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clusterManagerID}, result); err != nil {
		if kerrors.IsNotFound(err) {
			return nil, errors.HTTPNotFound().WithError(err)
		}

		return nil, errors.OAuth2ServerError("failed to get control plane").WithError(err)
	}

	return result, nil
}

// defaultApplicationBundle returns a default application bundle.
func (c *Client) defaultApplicationBundle(ctx context.Context, appclient appBundleLister) (*unikornv1.ClusterManagerApplicationBundle, error) {
	applicationBundles, err := appclient.ListClusterManager(ctx)
	if err != nil {
		return nil, errors.OAuth2ServerError("failed to list application bundles").WithError(err)
	}

	applicationBundles.Items = slices.DeleteFunc(applicationBundles.Items, func(bundle unikornv1.ClusterManagerApplicationBundle) bool {
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

	// Sort by semanitc version...
	slices.SortedStableFunc(slices.Values(applicationBundles.Items), func(a, b unikornv1.ClusterManagerApplicationBundle) int {
		return a.Spec.Version.Compare(&b.Spec.Version)
	})

	// ... and select the latest.
	return &applicationBundles.Items[len(applicationBundles.Items)-1], nil
}

// generate is a common function to create a Kubernetes type from an API one.
func (c *Client) generate(ctx context.Context, appclient appBundleLister, namespace *corev1.Namespace, organizationID, projectID string, request *openapi.ClusterManagerWrite) (*unikornv1.ClusterManager, error) {
	applicationBundle, err := c.defaultApplicationBundle(ctx, appclient)
	if err != nil {
		return nil, err
	}

	out := &unikornv1.ClusterManager{
		ObjectMeta: conversion.NewObjectMetadata(&request.Metadata, namespace.Name).WithOrganization(organizationID).WithProject(projectID).Get(),
		Spec: unikornv1.ClusterManagerSpec{
			Tags:                         conversion.GenerateTagList(request.Metadata.Tags),
			ApplicationBundle:            applicationBundle.Name,
			ApplicationBundleAutoUpgrade: &unikornv1.ApplicationBundleAutoUpgradeSpec{},
		},
	}

	if err := identitycommon.SetIdentityMetadata(ctx, &out.ObjectMeta); err != nil {
		return nil, errors.OAuth2ServerError("failed to set identity metadata").WithError(err)
	}

	return out, nil
}

// Create creates a control plane.
func (c *Client) create(ctx context.Context, appclient appBundleLister, organizationID, projectID string, request *openapi.ClusterManagerWrite) (*unikornv1.ClusterManager, error) {
	namespace, err := common.New(c.client).ProjectNamespace(ctx, organizationID, projectID)
	if err != nil {
		return nil, err
	}

	if namespace.DeletionTimestamp != nil {
		return nil, errors.OAuth2InvalidRequest("project is being deleted")
	}

	resource, err := c.generate(ctx, appclient, namespace, organizationID, projectID, request)
	if err != nil {
		return nil, err
	}

	if err := c.client.Create(ctx, resource); err != nil {
		// TODO: we can do a cached lookup to save the API traffic.
		if kerrors.IsAlreadyExists(err) {
			return nil, errors.HTTPConflict()
		}

		return nil, errors.OAuth2ServerError("failed to create control plane").WithError(err)
	}

	return resource, nil
}

func (c *Client) Create(ctx context.Context, appclient appBundleLister, organizationID, projectID string, request *openapi.ClusterManagerWrite) (*openapi.ClusterManagerRead, error) {
	result, err := c.create(ctx, appclient, organizationID, projectID, request)
	if err != nil {
		return nil, err
	}

	return c.convert(result), nil
}

// Delete deletes the control plane.
func (c *Client) Delete(ctx context.Context, organizationID, projectID, clusterManagerID string) error {
	namespace, err := common.New(c.client).ProjectNamespace(ctx, organizationID, projectID)
	if err != nil {
		return err
	}

	if namespace.DeletionTimestamp != nil {
		return errors.OAuth2InvalidRequest("project is being deleted")
	}

	controlPlane := &unikornv1.ClusterManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterManagerID,
			Namespace: namespace.Name,
		},
	}

	if err := c.client.Delete(ctx, controlPlane); err != nil {
		if kerrors.IsNotFound(err) {
			return errors.HTTPNotFound().WithError(err)
		}

		return errors.OAuth2ServerError("failed to delete control plane").WithError(err)
	}

	return nil
}

// Update implements read/modify/write for the control plane.
func (c *Client) Update(ctx context.Context, appclient appBundleLister, organizationID, projectID, clusterManagerID string, request *openapi.ClusterManagerWrite) error {
	namespace, err := common.New(c.client).ProjectNamespace(ctx, organizationID, projectID)
	if err != nil {
		return err
	}

	if namespace.DeletionTimestamp != nil {
		return errors.OAuth2InvalidRequest("project is being deleted")
	}

	current, err := c.get(ctx, namespace.Name, clusterManagerID)
	if err != nil {
		return err
	}

	required, err := c.generate(ctx, appclient, namespace, organizationID, projectID, request)
	if err != nil {
		return err
	}

	if err := conversion.UpdateObjectMetadata(required, current, identitycommon.IdentityMetadataMutator); err != nil {
		return errors.OAuth2ServerError("failed to merge metadata").WithError(err)
	}

	updated := current.DeepCopy()
	updated.Labels = required.Labels
	updated.Annotations = required.Annotations
	updated.Spec = required.Spec

	if err := c.client.Patch(ctx, updated, client.MergeFrom(current)); err != nil {
		return errors.OAuth2ServerError("failed to patch control plane").WithError(err)
	}

	return nil
}
