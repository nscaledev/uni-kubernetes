/*
Copyright 2025 the Unikorn Authors.
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

package client

import (
	"context"
	"net/http"

	coreclient "github.com/unikorn-cloud/core/pkg/client"
	baseclient "github.com/unikorn-cloud/identity/pkg/client"
	"github.com/unikorn-cloud/kubernetes/pkg/openapi"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Options = coreclient.HTTPOptions

// NewOptions must be used to create options for consistency.
func NewOptions() *Options {
	return coreclient.NewHTTPOptions("kubernetes")
}

// Client wraps up the raw OpenAPI client with things to make it useable e.g.
// authorization and TLS.
type Client struct {
	base *baseclient.BaseClient
}

// New creates a new client.
func New(client client.Client, options *Options, clientOptions *coreclient.HTTPClientOptions) *Client {
	return &Client{
		base: baseclient.NewBaseClient(client, options, clientOptions),
	}
}

// HTTPClient returns a new http client that will transparently do oauth2 header
// injection and refresh token updates.
func (c *Client) HTTPClient(ctx context.Context) (*http.Client, error) {
	return c.base.HTTPClient(ctx)
}

// APIClient returns a new OpenAPI client that can be used to access the API from another API.
func (c *Client) APIClient(ctx context.Context) (*openapi.ClientWithResponses, error) {
	return baseclient.APIClient(ctx, c.base, openapi.NewBuilder())
}

// ControllerClient returns a new OpenAPI client that can be used to access the API from another
// controller.  It requires a resource that stores the identity principal information.
func (c *Client) ControllerClient(ctx context.Context, resource metav1.Object) (*openapi.ClientWithResponses, error) {
	return baseclient.ControllerClient(ctx, c.base, openapi.NewBuilder(), resource)
}
