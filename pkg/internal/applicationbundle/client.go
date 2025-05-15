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

package applicationbundle

import (
	"context"
	"slices"

	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client wraps up application bundle related management handling.
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

func (c *Client) GetClusterManager(ctx context.Context, name string) (*unikornv1.ClusterManagerApplicationBundle, error) {
	result := &unikornv1.ClusterManagerApplicationBundle{}

	if err := c.client.Get(ctx, client.ObjectKey{Name: name}, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) GetKubernetesCluster(ctx context.Context, name string) (*unikornv1.KubernetesClusterApplicationBundle, error) {
	result := &unikornv1.KubernetesClusterApplicationBundle{}

	if err := c.client.Get(ctx, client.ObjectKey{Name: name}, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) GetVirtualKubernetesCluster(ctx context.Context, name string) (*unikornv1.VirtualKubernetesClusterApplicationBundle, error) {
	result := &unikornv1.VirtualKubernetesClusterApplicationBundle{}

	if err := c.client.Get(ctx, client.ObjectKey{Name: name}, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) ListClusterManager(ctx context.Context) (*unikornv1.ClusterManagerApplicationBundleList, error) {
	result := &unikornv1.ClusterManagerApplicationBundleList{}

	if err := c.client.List(ctx, result); err != nil {
		return nil, err
	}

	slices.SortStableFunc(result.Items, unikornv1.CompareClusterManagerApplicationBundle)

	return result, nil
}

func (c *Client) ListCluster(ctx context.Context) (*unikornv1.KubernetesClusterApplicationBundleList, error) {
	result := &unikornv1.KubernetesClusterApplicationBundleList{}

	if err := c.client.List(ctx, result); err != nil {
		return nil, err
	}

	slices.SortStableFunc(result.Items, unikornv1.CompareKubernetesClusterApplicationBundle)

	return result, nil
}

func (c *Client) ListVirtualCluster(ctx context.Context) (*unikornv1.VirtualKubernetesClusterApplicationBundleList, error) {
	result := &unikornv1.VirtualKubernetesClusterApplicationBundleList{}

	if err := c.client.List(ctx, result); err != nil {
		return nil, err
	}

	slices.SortStableFunc(result.Items, unikornv1.CompareVirtualKubernetesClusterApplicationBundle)

	return result, nil
}
