/*
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

package common

import (
	"context"
	"errors"

	"github.com/unikorn-cloud/core/pkg/constants"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrNamespace = errors.New("unable to resolve project namespace")
)

// Client wraps up control plane related management handling.
type Client struct {
	// client allows Kubernetes API access.
	client client.Client
}

// New returns a new client with required parameters.
func New(client client.Client) *Client {
	return &Client{
		client: client,
	}
}

func (c *Client) ProjectNamespace(ctx context.Context, organization, project string) (*corev1.Namespace, error) {
	organizationRequirement, err := labels.NewRequirement(constants.OrganizationLabel, selection.Equals, []string{organization})
	if err != nil {
		return nil, err
	}

	projectRequirement, err := labels.NewRequirement(constants.ProjectLabel, selection.Equals, []string{project})
	if err != nil {
		return nil, err
	}

	kindRequirement, err := labels.NewRequirement(constants.KindLabel, selection.Equals, []string{constants.KindLabelValueProject})
	if err != nil {
		return nil, err
	}

	selector := labels.NewSelector()
	selector = selector.Add(*organizationRequirement, *projectRequirement, *kindRequirement)

	options := &client.ListOptions{
		LabelSelector: selector,
	}

	var namespaces corev1.NamespaceList

	if err := c.client.List(ctx, &namespaces, options); err != nil {
		return nil, err
	}

	if len(namespaces.Items) != 1 {
		return nil, ErrNamespace
	}

	return &namespaces.Items[0], nil
}
