//go:build integration

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

package identity_test

import (
	"context"
	"fmt"
	"net"
	"testing"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/pact-foundation/pact-go/v2/models"

	coreclient "github.com/unikorn-cloud/core/pkg/openapi"
	contract "github.com/unikorn-cloud/core/pkg/testing/contract"
	identityapi "github.com/unikorn-cloud/identity/pkg/openapi"
)

var testingT *testing.T //nolint:gochecknoglobals

func TestContracts(t *testing.T) { //nolint:paralleltest
	testingT = t

	RegisterFailHandler(Fail)
	RunSpecs(t, "Identity Consumer Contract Suite")
}

func createIdentityClient(config consumer.MockServerConfig) (*identityapi.ClientWithResponses, error) {
	url := fmt.Sprintf("http://%s", net.JoinHostPort(config.Host, fmt.Sprintf("%d", config.Port)))

	return identityapi.NewClientWithResponses(url)
}

// allocationTestParams holds parameters for allocation tests.
type allocationTestParams struct {
	organizationID string
	projectID      string
	resourceID     string
	name           string
	kind           string
	allocationKind string
	description    string
}

// setupCreateAllocationInteraction configures the pact interaction for creating an allocation.
func setupCreateAllocationInteraction(pact *consumer.V4HTTPMockProvider, params allocationTestParams) {
	pact.AddInteraction().
		GivenWithParameter(models.ProviderState{
			Name: "project exists",
			Parameters: map[string]interface{}{
				"organizationID": params.organizationID,
				"projectID":      params.projectID,
			},
		}).
		UponReceiving(params.description).
		WithRequest("POST", fmt.Sprintf("/api/v1/organizations/%s/projects/%s/allocations", params.organizationID, params.projectID), func(b *consumer.V4RequestBuilder) {
			b.JSONBody(map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": matchers.String(params.name),
				},
				"spec": map[string]interface{}{
					"id":   matchers.String(params.resourceID),
					"kind": matchers.String(params.kind),
					"allocations": []map[string]interface{}{
						{
							"kind":      matchers.String(params.allocationKind),
							"committed": matchers.Integer(1),
							"reserved":  matchers.Integer(0),
						},
						{
							"kind":      matchers.String("servers"),
							"committed": matchers.Integer(3),
							"reserved":  matchers.Integer(10),
						},
						{
							"kind":      matchers.String("gpus"),
							"committed": matchers.Integer(0),
							"reserved":  matchers.Integer(0),
						},
					},
				},
			})
		}).
		WillRespondWith(201, func(b *consumer.V4ResponseBuilder) {
			b.JSONBody(map[string]interface{}{
				"metadata": map[string]interface{}{
					"id":                 matchers.UUID(),
					"name":               matchers.String(params.name),
					"organizationId":     matchers.String(params.organizationID),
					"projectId":          matchers.String(params.projectID),
					"creationTime":       matchers.Timestamp(),
					"provisioningStatus": matchers.String("provisioned"),
				},
				"spec": map[string]interface{}{
					"id":   matchers.String(params.resourceID),
					"kind": matchers.String(params.kind),
					"allocations": []map[string]interface{}{
						{
							"kind":      matchers.String(params.allocationKind),
							"committed": matchers.Integer(1),
							"reserved":  matchers.Integer(0),
						},
						{
							"kind":      matchers.String("servers"),
							"committed": matchers.Integer(3),
							"reserved":  matchers.Integer(10),
						},
						{
							"kind":      matchers.String("gpus"),
							"committed": matchers.Integer(0),
							"reserved":  matchers.Integer(0),
						},
					},
				},
			})
		})
}

// createAllocationTestFunc returns a test function for creating an allocation.
func createAllocationTestFunc(ctx context.Context, params allocationTestParams) func(consumer.MockServerConfig) error {
	return func(config consumer.MockServerConfig) error {
		identityClient, err := createIdentityClient(config)
		if err != nil {
			return fmt.Errorf("creating identity client: %w", err)
		}

		allocationReq := identityapi.AllocationWrite{
			Metadata: coreclient.ResourceWriteMetadata{
				Name: params.name,
			},
			Spec: identityapi.AllocationSpec{
				Id:   params.resourceID,
				Kind: params.kind,
				Allocations: identityapi.ResourceAllocationList{
					{
						Kind:      params.allocationKind,
						Committed: 1,
						Reserved:  0,
					},
					{
						Kind:      "servers",
						Committed: 3,
						Reserved:  10,
					},
					{
						Kind:      "gpus",
						Committed: 0,
						Reserved:  0,
					},
				},
			},
		}

		resp, err := identityClient.PostApiV1OrganizationsOrganizationIDProjectsProjectIDAllocationsWithResponse(
			ctx, params.organizationID, params.projectID, allocationReq)
		if err != nil {
			return fmt.Errorf("creating allocation: %w", err)
		}

		Expect(resp.StatusCode()).To(Equal(201))
		Expect(resp.JSON201).NotTo(BeNil())
		Expect(resp.JSON201.Spec.Id).To(Equal(params.resourceID))
		Expect(resp.JSON201.Metadata.OrganizationId).To(Equal(params.organizationID))
		Expect(resp.JSON201.Metadata.ProjectId).To(Equal(params.projectID))

		return nil
	}
}

var _ = Describe("Identity Service Contract", func() {
	var (
		pact *consumer.V4HTTPMockProvider
		ctx  context.Context
	)

	BeforeEach(func() {
		var err error
		pact, err = contract.NewV4Pact(contract.PactConfig{
			Consumer: "uni-kubernetes",
			Provider: "uni-identity",
			PactDir:  "./pacts",
		})
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	Describe("ResourceAllocations", func() {
		Context("when creating a kubernetes cluster allocation", func() {
			It("creates allocation for kubernetes cluster resources", func() {
				params := allocationTestParams{
					organizationID: "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
					projectID:      "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e",
					resourceID:     "c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f",
					name:           "undefined",
					kind:           "kubernetescluster",
					allocationKind: "clusters",
					description:    "a request to create kubernetes cluster allocation",
				}

				setupCreateAllocationInteraction(pact, params)
				Expect(pact.ExecuteTest(testingT, createAllocationTestFunc(ctx, params))).To(Succeed())
			})
		})

		Context("when creating a virtual cluster allocation", func() {
			It("creates allocation for virtual cluster resources", func() {
				params := allocationTestParams{
					organizationID: "d4e5f6a7-b8c9-4d0e-1f2a-3b4c5d6e7f8a",
					projectID:      "e5f6a7b8-c9d0-4e1f-2a3b-4c5d6e7f8a9b",
					resourceID:     "f6a7b8c9-d0e1-4f2a-3b4c-5d6e7f8a9b0c",
					name:           "undefined",
					kind:           "virtualcluster",
					allocationKind: "clusters",
					description:    "a request to create virtual cluster allocation",
				}

				setupCreateAllocationInteraction(pact, params)
				Expect(pact.ExecuteTest(testingT, createAllocationTestFunc(ctx, params))).To(Succeed())
			})
		})

		Context("when updating an allocation", func() {
			It("updates allocation with new resource counts", func() {
				organizationID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
				projectID := "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"
				allocationID := "c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f"
				resourceID := "d4e5f6a7-b8c9-4d0e-1f2a-3b4c5d6e7f8a"

				pact.AddInteraction().
					GivenWithParameter(models.ProviderState{
						Name: "allocation exists",
						Parameters: map[string]interface{}{
							"organizationID": organizationID,
							"projectID":      projectID,
							"allocationID":   allocationID,
						},
					}).
					UponReceiving("a request to update kubernetes cluster allocation").
					WithRequest("PUT",
						fmt.Sprintf("/api/v1/organizations/%s/projects/%s/allocations/%s",
							organizationID, projectID, allocationID), func(b *consumer.V4RequestBuilder) {
							b.JSONBody(map[string]interface{}{
								"metadata": map[string]interface{}{
									"name": matchers.String("undefined"),
								},
								"spec": map[string]interface{}{
									"id":   matchers.String(resourceID),
									"kind": matchers.String("kubernetescluster"),
									"allocations": []map[string]interface{}{
										{
											"kind":      matchers.String("clusters"),
											"committed": matchers.Integer(1),
											"reserved":  matchers.Integer(0),
										},
										{
											"kind":      matchers.String("servers"),
											"committed": matchers.Integer(5),
											"reserved":  matchers.Integer(15),
										},
										{
											"kind":      matchers.String("gpus"),
											"committed": matchers.Integer(5),
											"reserved":  matchers.Integer(10),
										},
									},
								},
							})
						}).
					WillRespondWith(200, func(b *consumer.V4ResponseBuilder) {
						b.JSONBody(map[string]interface{}{
							"metadata": map[string]interface{}{
								"id":                 matchers.String(allocationID),
								"name":               matchers.String("undefined"),
								"organizationId":     matchers.String(organizationID),
								"projectId":          matchers.String(projectID),
								"creationTime":       matchers.Timestamp(),
								"provisioningStatus": matchers.String("provisioned"),
							},
							"spec": map[string]interface{}{
								"id":   matchers.String(resourceID),
								"kind": matchers.String("kubernetescluster"),
								"allocations": []map[string]interface{}{
									{
										"kind":      matchers.String("clusters"),
										"committed": matchers.Integer(1),
										"reserved":  matchers.Integer(0),
									},
									{
										"kind":      matchers.String("servers"),
										"committed": matchers.Integer(5),
										"reserved":  matchers.Integer(15),
									},
									{
										"kind":      matchers.String("gpus"),
										"committed": matchers.Integer(5),
										"reserved":  matchers.Integer(10),
									},
								},
							},
						})
					})

				test := func(config consumer.MockServerConfig) error {
					identityClient, err := createIdentityClient(config)
					if err != nil {
						return fmt.Errorf("creating identity client: %w", err)
					}

					allocationReq := identityapi.AllocationWrite{
						Metadata: coreclient.ResourceWriteMetadata{
							Name: "undefined",
						},
						Spec: identityapi.AllocationSpec{
							Id:   resourceID,
							Kind: "kubernetescluster",
							Allocations: identityapi.ResourceAllocationList{
								{
									Kind:      "clusters",
									Committed: 1,
									Reserved:  0,
								},
								{
									Kind:      "servers",
									Committed: 5,
									Reserved:  15,
								},
								{
									Kind:      "gpus",
									Committed: 5,
									Reserved:  10,
								},
							},
						},
					}

					resp, err := identityClient.PutApiV1OrganizationsOrganizationIDProjectsProjectIDAllocationsAllocationIDWithResponse(
						ctx, organizationID, projectID, allocationID, allocationReq)
					if err != nil {
						return fmt.Errorf("updating allocation: %w", err)
					}

					Expect(resp.StatusCode()).To(Equal(200))
					Expect(resp.JSON200).NotTo(BeNil())
					Expect(resp.JSON200.Spec.Id).To(Equal(resourceID))
					Expect(resp.JSON200.Metadata.Id).To(Equal(allocationID))

					return nil
				}

				Expect(pact.ExecuteTest(testingT, test)).To(Succeed())
			})
		})

		Context("when deleting an allocation", func() {
			It("removes allocation successfully", func() {
				organizationID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
				projectID := "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"
				allocationID := "c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f"

				pact.AddInteraction().
					GivenWithParameter(models.ProviderState{
						Name: "allocation exists",
						Parameters: map[string]interface{}{
							"organizationID": organizationID,
							"projectID":      projectID,
							"allocationID":   allocationID,
						},
					}).
					UponReceiving("a request to delete allocation").
					WithRequest("DELETE",
						fmt.Sprintf("/api/v1/organizations/%s/projects/%s/allocations/%s",
							organizationID, projectID, allocationID)).
					WillRespondWith(202)

				test := func(config consumer.MockServerConfig) error {
					identityClient, err := createIdentityClient(config)
					if err != nil {
						return fmt.Errorf("creating identity client: %w", err)
					}

					resp, err := identityClient.DeleteApiV1OrganizationsOrganizationIDProjectsProjectIDAllocationsAllocationIDWithResponse(
						ctx, organizationID, projectID, allocationID)
					if err != nil {
						return fmt.Errorf("deleting allocation: %w", err)
					}

					Expect(resp.StatusCode()).To(Equal(202))

					return nil
				}

				Expect(pact.ExecuteTest(testingT, test)).To(Succeed())
			})
		})
	})
})
