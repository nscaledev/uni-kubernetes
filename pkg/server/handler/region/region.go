/*
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

package region

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	servererrors "github.com/unikorn-cloud/core/pkg/server/errors"
	"github.com/unikorn-cloud/core/pkg/util/cache"
	"github.com/unikorn-cloud/kubernetes/pkg/openapi"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

var (
	ErrUnhandled = errors.New("unhandled case")
)

const (
	defaultCacheSize = 4096
)

// regionTypeFilter creates a filter for use with DeleteFunc that selects regions
// based on the requested cluster type.
func regionTypeFilter(t openapi.RegionTypeParameter) (func(regionapi.RegionRead) bool, error) {
	switch t {
	case openapi.Physical:
		return func(x regionapi.RegionRead) bool { return x.Spec.Type == regionapi.RegionTypeKubernetes }, nil
	case openapi.Virtual:
		return func(x regionapi.RegionRead) bool { return x.Spec.Type != regionapi.RegionTypeKubernetes }, nil
	}

	return nil, ErrUnhandled
}

// Client provides a caching layer for retrieval of region assets, and lazy population.
type Client struct {
	client      regionapi.ClientWithResponsesInterface
	regionCache *cache.LRUExpireCache[string, []regionapi.RegionRead]
	flavorCache *cache.LRUExpireCache[string, []regionapi.Flavor]
	imageCache  *cache.LRUExpireCache[string, []regionapi.Image]
}

var _ ClientInterface = &Client{}

// New returns a new client.
func New(client regionapi.ClientWithResponsesInterface) *Client {
	return &Client{
		client:      client,
		regionCache: cache.NewLRUExpireCache[string, []regionapi.RegionRead](defaultCacheSize),
		flavorCache: cache.NewLRUExpireCache[string, []regionapi.Flavor](defaultCacheSize),
		imageCache:  cache.NewLRUExpireCache[string, []regionapi.Image](defaultCacheSize),
	}
}

// Client returns a client.
func (c *Client) Client() regionapi.ClientWithResponsesInterface {
	return c.client
}

// Get gets a specific region.
func (c *Client) Get(ctx context.Context, organizationID, regionID string) (*regionapi.RegionDetailRead, error) {
	// TODO: Danger, danger, this returns possible sensitive information that must not
	// be leaked.  Add the correct API.
	resp, err := c.client.GetApiV1OrganizationsOrganizationIDRegionsRegionIDDetailWithResponse(ctx, organizationID, regionID)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, servererrors.PropagateError(resp.HTTPResponse, resp)
	}

	return resp.JSON200, nil
}

func (c *Client) list(ctx context.Context, organizationID string) ([]regionapi.RegionRead, error) {
	if regions, ok := c.regionCache.Get(organizationID); ok {
		return regions, nil
	}

	resp, err := c.client.GetApiV1OrganizationsOrganizationIDRegionsWithResponse(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, servererrors.PropagateError(resp.HTTPResponse, resp)
	}

	regions := *resp.JSON200

	c.regionCache.Add(organizationID, regions, time.Hour)

	return regions, nil
}

// List lists all regions.
func (c *Client) List(ctx context.Context, organizationID string, params openapi.GetApiV1OrganizationsOrganizationIDRegionsParams) ([]regionapi.RegionRead, error) {
	regions, err := c.list(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	filter, err := regionTypeFilter(params.RegionType)
	if err != nil {
		return nil, err
	}

	return slices.DeleteFunc(regions, filter), nil
}

// Flavors returns all Kubernetes compatible flavors.
func (c *Client) Flavors(ctx context.Context, organizationID, regionID string) ([]regionapi.Flavor, error) {
	cacheKey := organizationID + ":" + regionID

	if flavors, ok := c.flavorCache.Get(cacheKey); ok {
		return flavors, nil
	}

	resp, err := c.client.GetApiV1OrganizationsOrganizationIDRegionsRegionIDFlavorsWithResponse(ctx, organizationID, regionID)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, servererrors.PropagateError(resp.HTTPResponse, resp)
	}

	flavors := *resp.JSON200

	flavors = slices.DeleteFunc(flavors, func(x regionapi.Flavor) bool {
		// kubeadm requires at least 2 VCPU and 2 GiB memory.
		return x.Spec.Cpus < 2 || x.Spec.Memory < 2
	})

	c.flavorCache.Add(cacheKey, flavors, time.Hour)

	return flavors, nil
}

// Images returns all Kubernetes compatible images.
func (c *Client) Images(ctx context.Context, organizationID, regionID string) ([]regionapi.Image, error) {
	cacheKey := organizationID + ":" + regionID

	if images, ok := c.imageCache.Get(cacheKey); ok {
		return images, nil
	}

	resp, err := c.client.GetApiV1OrganizationsOrganizationIDRegionsRegionIDImagesWithResponse(ctx, organizationID, regionID)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, servererrors.PropagateError(resp.HTTPResponse, resp)
	}

	images := *resp.JSON200

	images = slices.DeleteFunc(images, func(x regionapi.Image) bool {
		return x.Spec.SoftwareVersions == nil || (*x.Spec.SoftwareVersions)["kubernetes"] == ""
	})

	c.imageCache.Add(cacheKey, images, time.Hour)

	return images, nil
}
