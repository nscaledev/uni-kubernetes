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

package region

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	coreclient "github.com/unikorn-cloud/core/pkg/client"
	coreapiutils "github.com/unikorn-cloud/core/pkg/util/api"
	identityclient "github.com/unikorn-cloud/identity/pkg/client"
	"github.com/unikorn-cloud/kubernetes/pkg/constants"
	regionclient "github.com/unikorn-cloud/region/pkg/client"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrResourceDependency = errors.New("resource dependency error")
)

// ControllerClient creates a new authenticated client and context for use against the region service.
func ControllerClient(ctx context.Context, cli client.Client, httpOptions *coreclient.HTTPClientOptions, identityOptions *identityclient.Options, regionOptions *regionclient.Options, traceName string, resource metav1.Object) (context.Context, regionapi.ClientWithResponsesInterface, error) {
	tokenIssuer := identityclient.NewTokenIssuer(cli, identityOptions, httpOptions, constants.Application, constants.Version)

	token, err := tokenIssuer.Issue(ctx, traceName)
	if err != nil {
		return nil, nil, err
	}

	getter := regionclient.New(cli, regionOptions, httpOptions)

	client, err := getter.ControllerClient(ctx, token, resource)
	if err != nil {
		return nil, nil, err
	}

	return ctx, client, nil
}

// Region returns the chosen region by ID.
func Region(ctx context.Context, client regionapi.ClientWithResponsesInterface, organizationID, regionID string) (*regionapi.RegionDetailRead, error) {
	response, err := client.GetApiV1OrganizationsOrganizationIDRegionsRegionIDDetailWithResponse(ctx, organizationID, regionID)
	if err != nil {
		return nil, err
	}

	if response.StatusCode() != http.StatusOK {
		return nil, coreapiutils.ExtractError(response.StatusCode(), response)
	}

	return response.JSON200, nil
}

// Kubeconfig returns the region's Kubernetes config.
func Kubeconfig(region *regionapi.RegionDetailRead) ([]byte, error) {
	if region.Spec.Type != regionapi.Kubernetes {
		return nil, fmt.Errorf("%w: requested region of incorrect type", ErrResourceDependency)
	}

	if region.Spec.Kubernetes == nil || region.Spec.Kubernetes.Kubeconfig == "" {
		return nil, fmt.Errorf("%w: requested region missing kubeconfig", ErrResourceDependency)
	}

	kubeConfig, err := base64.RawURLEncoding.DecodeString(region.Spec.Kubernetes.Kubeconfig)
	if err != nil {
		return nil, err
	}

	return kubeConfig, nil
}
