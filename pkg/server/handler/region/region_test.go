/*
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

package region_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	kubernetesapi "github.com/unikorn-cloud/kubernetes/pkg/openapi"
	"github.com/unikorn-cloud/kubernetes/pkg/server/handler/region"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

var errUnexpectedRequestCount = errors.New("unexpected request count")

func TestListDoesNotCacheImpersonatedResponses(t *testing.T) {
	t.Parallel()

	requests := 0

	client, err := newTestClient(func(req *http.Request) (*http.Response, error) {
		requests++

		switch requests {
		case 1:
			return jsonResponse([]map[string]any{
				{
					"metadata": map[string]any{"id": "region-1"},
					"spec":     map[string]any{"type": "kubernetes"},
				},
			})
		case 2:
			return jsonResponse([]map[string]any{
				{
					"metadata": map[string]any{"id": "region-2"},
					"spec":     map[string]any{"type": "kubernetes"},
				},
			})
		default:
			t.Fatalf("unexpected request count: %d", requests)

			return nil, errUnexpectedRequestCount
		}
	})
	require.NoError(t, err)

	regions := region.New(client)
	params := kubernetesapi.GetApiV1OrganizationsOrganizationIDRegionsParams{
		RegionType: kubernetesapi.Virtual,
	}

	first, err := regions.List(t.Context(), "org-1", params)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, "region-1", first[0].Metadata.Id)

	second, err := regions.List(t.Context(), "org-1", params)
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Equal(t, "region-2", second[0].Metadata.Id)
	require.Equal(t, 2, requests)
}

func TestFlavorsDoesNotCacheImpersonatedResponses(t *testing.T) {
	t.Parallel()

	requests := 0

	client, err := newTestClient(func(req *http.Request) (*http.Response, error) {
		requests++

		switch requests {
		case 1:
			return jsonResponse([]map[string]any{
				{
					"metadata": map[string]any{"id": "flavor-1"},
					"spec":     map[string]any{"cpus": 2, "memory": 4},
				},
			})
		case 2:
			return jsonResponse([]map[string]any{
				{
					"metadata": map[string]any{"id": "flavor-2"},
					"spec":     map[string]any{"cpus": 2, "memory": 4},
				},
			})
		default:
			t.Fatalf("unexpected request count: %d", requests)

			return nil, errUnexpectedRequestCount
		}
	})
	require.NoError(t, err)

	regions := region.New(client)

	first, err := regions.Flavors(t.Context(), "org-1", "region-1")
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, "flavor-1", first[0].Metadata.Id)

	second, err := regions.Flavors(t.Context(), "org-1", "region-1")
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Equal(t, "flavor-2", second[0].Metadata.Id)
	require.Equal(t, 2, requests)
}

func TestImagesDoesNotCacheImpersonatedResponses(t *testing.T) {
	t.Parallel()

	requests := 0

	client, err := newTestClient(func(req *http.Request) (*http.Response, error) {
		requests++

		switch requests {
		case 1:
			return jsonResponse([]map[string]any{
				{
					"metadata": map[string]any{"id": "image-1"},
					"spec": map[string]any{
						"softwareVersions": map[string]any{"kubernetes": "1.31.0"},
					},
				},
			})
		case 2:
			return jsonResponse([]map[string]any{
				{
					"metadata": map[string]any{"id": "image-2"},
					"spec": map[string]any{
						"softwareVersions": map[string]any{"kubernetes": "1.31.1"},
					},
				},
			})
		default:
			t.Fatalf("unexpected request count: %d", requests)

			return nil, errUnexpectedRequestCount
		}
	})
	require.NoError(t, err)

	regions := region.New(client)

	first, err := regions.Images(t.Context(), "org-1", "region-1")
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, "image-1", first[0].Metadata.Id)

	second, err := regions.Images(t.Context(), "org-1", "region-1")
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Equal(t, "image-2", second[0].Metadata.Id)
	require.Equal(t, 2, requests)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestClient(fn roundTripFunc) (*regionapi.ClientWithResponses, error) {
	httpClient := &http.Client{
		Transport: fn,
	}

	return regionapi.NewClientWithResponses("http://region.example", regionapi.WithHTTPClient(httpClient))
}

func jsonResponse(body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(bytes.NewReader(data)),
	}, nil
}
