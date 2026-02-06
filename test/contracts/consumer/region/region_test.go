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

package region_test

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
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

var testingT *testing.T //nolint:gochecknoglobals

func TestContracts(t *testing.T) { //nolint:paralleltest
	testingT = t

	RegisterFailHandler(Fail)
	RunSpecs(t, "Region Consumer Contract Suite")
}

func createRegionClient(config consumer.MockServerConfig) (*regionapi.ClientWithResponses, error) {
	url := fmt.Sprintf("http://%s", net.JoinHostPort(config.Host, fmt.Sprintf("%d", config.Port)))

	return regionapi.NewClientWithResponses(url)
}

var _ = Describe("Region Service Contract", func() {
	var (
		pact *consumer.V4HTTPMockProvider
		ctx  context.Context
	)

	BeforeEach(func() {
		var err error
		pact, err = contract.NewV4Pact(contract.PactConfig{
			Consumer: "uni-kubernetes",
			Provider: "uni-region",
			PactDir:  "./pacts",
		})
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	Describe("Region Information", func() {
		Context("when getting region detail", func() {
			It("retrieves region configuration with features", func() {
				organizationID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
				regionID := "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"

				pact.AddInteraction().
					GivenWithParameter(models.ProviderState{
						Name: "region exists",
						Parameters: map[string]interface{}{
							"organizationID": organizationID,
							"regionID":       regionID,
						},
					}).
					UponReceiving("a request to get region detail").
					WithRequest("GET", fmt.Sprintf("/api/v1/organizations/%s/regions/%s/detail", organizationID, regionID)).
					WillRespondWith(200, func(b *consumer.V4ResponseBuilder) {
						b.JSONBody(map[string]interface{}{
							"metadata": map[string]interface{}{
								"id":           matchers.UUID(),
								"name":         matchers.String("us-west-2"),
								"creationTime": matchers.Timestamp(),
							},
							"spec": map[string]interface{}{
								"type": matchers.String("openstack"),
								"features": map[string]interface{}{
									"physicalNetworks": true,
								},
							},
						})
					})

				test := func(config consumer.MockServerConfig) error {
					regionClient, err := createRegionClient(config)
					if err != nil {
						return fmt.Errorf("creating region client: %w", err)
					}

					resp, err := regionClient.GetApiV1OrganizationsOrganizationIDRegionsRegionIDDetailWithResponse(
						ctx, organizationID, regionID)
					if err != nil {
						return fmt.Errorf("getting region detail: %w", err)
					}

					Expect(resp.StatusCode()).To(Equal(200))
					Expect(resp.JSON200).NotTo(BeNil())

					return nil
				}

				Expect(pact.ExecuteTest(testingT, test)).To(Succeed())
			})
		})

		Context("when listing regions", func() {
			It("retrieves all accessible regions", func() {
				organizationID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"

				pact.AddInteraction().
					GivenWithParameter(models.ProviderState{
						Name: "organization has regions",
						Parameters: map[string]interface{}{
							"organizationID": organizationID,
						},
					}).
					UponReceiving("a request to list regions").
					WithRequest("GET", fmt.Sprintf("/api/v1/organizations/%s/regions", organizationID)).
					WillRespondWith(200, func(b *consumer.V4ResponseBuilder) {
						b.JSONBody(matchers.EachLike(map[string]interface{}{
							"metadata": map[string]interface{}{
								"id":           matchers.UUID(),
								"name":         matchers.String("us-west-2"),
								"creationTime": matchers.Timestamp(),
							},
							"spec": map[string]interface{}{
								"type": matchers.String("openstack"),
							},
						}, 1))
					})

				test := func(config consumer.MockServerConfig) error {
					regionClient, err := createRegionClient(config)
					if err != nil {
						return fmt.Errorf("creating region client: %w", err)
					}

					resp, err := regionClient.GetApiV1OrganizationsOrganizationIDRegionsWithResponse(
						ctx, organizationID)
					if err != nil {
						return fmt.Errorf("listing regions: %w", err)
					}

					Expect(resp.StatusCode()).To(Equal(200))
					Expect(resp.JSON200).NotTo(BeNil())
					Expect(*resp.JSON200).NotTo(BeEmpty())
					Expect((*resp.JSON200)[0].Spec.Type).To(Equal(regionapi.RegionTypeOpenstack))

					return nil
				}

				Expect(pact.ExecuteTest(testingT, test)).To(Succeed())
			})
		})
	})

	Describe("Flavors and Images", func() {
		Context("when listing flavors", func() {
			It("retrieves available VM flavors for cluster provisioning", func() {
				organizationID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
				regionID := "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"

				pact.AddInteraction().
					GivenWithParameter(models.ProviderState{
						Name: "region has flavors",
						Parameters: map[string]interface{}{
							"organizationID": organizationID,
							"regionID":       regionID,
						},
					}).
					UponReceiving("a request to list flavors").
					WithRequest("GET", fmt.Sprintf("/api/v1/organizations/%s/regions/%s/flavors", organizationID, regionID)).
					WillRespondWith(200, func(b *consumer.V4ResponseBuilder) {
						b.JSONBody(matchers.EachLike(map[string]interface{}{
							"metadata": map[string]interface{}{
								"id":   matchers.UUID(),
								"name": matchers.String("m1.large"),
							},
							"spec": map[string]interface{}{
								"cpus":   matchers.Integer(4),
								"memory": matchers.Integer(8192),
							},
						}, 1))
					})

				test := func(config consumer.MockServerConfig) error {
					regionClient, err := createRegionClient(config)
					if err != nil {
						return fmt.Errorf("creating region client: %w", err)
					}

					resp, err := regionClient.GetApiV1OrganizationsOrganizationIDRegionsRegionIDFlavorsWithResponse(
						ctx, organizationID, regionID)
					if err != nil {
						return fmt.Errorf("listing flavors: %w", err)
					}

					Expect(resp.StatusCode()).To(Equal(200))
					Expect(resp.JSON200).NotTo(BeNil())
					Expect(*resp.JSON200).NotTo(BeEmpty())

					return nil
				}

				Expect(pact.ExecuteTest(testingT, test)).To(Succeed())
			})
		})

		Context("when listing images", func() {
			It("retrieves available OS images for cluster provisioning", func() {
				organizationID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
				regionID := "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"

				pact.AddInteraction().
					GivenWithParameter(models.ProviderState{
						Name: "region has images",
						Parameters: map[string]interface{}{
							"organizationID": organizationID,
							"regionID":       regionID,
						},
					}).
					UponReceiving("a request to list images").
					WithRequest("GET", fmt.Sprintf("/api/v1/organizations/%s/regions/%s/images", organizationID, regionID)).
					WillRespondWith(200, func(b *consumer.V4ResponseBuilder) {
						b.JSONBody(matchers.EachLike(map[string]interface{}{
							"metadata": map[string]interface{}{
								"id":   matchers.UUID(),
								"name": matchers.String("Ubuntu 22.04 Kubernetes v1.27.0"),
							},
							"spec": map[string]interface{}{
								"softwareVersions": map[string]interface{}{
									"kubernetes": matchers.String("v1.27.0"),
								},
							},
						}, 1))
					})

				test := func(config consumer.MockServerConfig) error {
					regionClient, err := createRegionClient(config)
					if err != nil {
						return fmt.Errorf("creating region client: %w", err)
					}

					resp, err := regionClient.GetApiV1OrganizationsOrganizationIDRegionsRegionIDImagesWithResponse(
						ctx, organizationID, regionID)
					if err != nil {
						return fmt.Errorf("listing images: %w", err)
					}

					Expect(resp.StatusCode()).To(Equal(200))
					Expect(resp.JSON200).NotTo(BeNil())
					Expect(*resp.JSON200).NotTo(BeEmpty())

					return nil
				}

				Expect(pact.ExecuteTest(testingT, test)).To(Succeed())
			})
		})
	})

	Describe("Identity Management", func() {
		Context("when creating an identity", func() {
			It("creates cloud credentials for cluster provisioning", func() {
				organizationID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
				projectID := "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"
				regionID := "c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f"

				pact.AddInteraction().
					GivenWithParameter(models.ProviderState{
						Name: "project exists in region",
						Parameters: map[string]interface{}{
							"organizationID": organizationID,
							"projectID":      projectID,
							"regionID":       regionID,
						},
					}).
					UponReceiving("a request to create identity for kubernetes cluster").
					WithRequest("POST", fmt.Sprintf("/api/v1/organizations/%s/projects/%s/identities", organizationID, projectID), func(b *consumer.V4RequestBuilder) {
						b.JSONBody(map[string]interface{}{
							"metadata": map[string]interface{}{
								"name":        matchers.String("kubernetes-cluster-test"),
								"description": matchers.String("Identity for Kubernetes cluster"),
							},
							"spec": map[string]interface{}{
								"regionId": matchers.String(regionID),
							},
						})
					}).
					WillRespondWith(201, func(b *consumer.V4ResponseBuilder) {
						b.JSONBody(map[string]interface{}{
							"metadata": map[string]interface{}{
								"id":                 matchers.UUID(),
								"name":               matchers.String("kubernetes-cluster-test"),
								"organizationId":     matchers.UUID(),
								"projectId":          matchers.UUID(),
								"creationTime":       matchers.Timestamp(),
								"provisioningStatus": matchers.String("unknown"),
							},
							"spec": map[string]interface{}{
								"regionId": matchers.UUID(),
							},
						})
					})

				test := func(config consumer.MockServerConfig) error {
					regionClient, err := createRegionClient(config)
					if err != nil {
						return fmt.Errorf("creating region client: %w", err)
					}

					identityReq := regionapi.IdentityWrite{
						Metadata: coreclient.ResourceWriteMetadata{
							Name:        "kubernetes-cluster-test",
							Description: func() *string { s := "Identity for Kubernetes cluster"; return &s }(),
						},
						Spec: regionapi.IdentityWriteSpec{
							RegionId: regionID,
						},
					}

					resp, err := regionClient.PostApiV1OrganizationsOrganizationIDProjectsProjectIDIdentitiesWithResponse(
						ctx, organizationID, projectID, identityReq)
					if err != nil {
						return fmt.Errorf("creating identity: %w", err)
					}

					Expect(resp.StatusCode()).To(Equal(201))
					Expect(resp.JSON201).NotTo(BeNil())

					return nil
				}

				Expect(pact.ExecuteTest(testingT, test)).To(Succeed())
			})
		})

		Context("when getting an identity", func() {
			It("retrieves identity with cloud credentials and provisioning status", func() {
				organizationID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
				projectID := "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"
				identityID := "c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f"

				pact.AddInteraction().
					GivenWithParameter(models.ProviderState{
						Name: "identity is provisioned",
						Parameters: map[string]interface{}{
							"organizationID": organizationID,
							"projectID":      projectID,
							"identityID":     identityID,
							"regionType":     "openstack",
						},
					}).
					UponReceiving("a request to get provisioned identity").
					WithRequest("GET", fmt.Sprintf("/api/v1/organizations/%s/projects/%s/identities/%s",
						organizationID, projectID, identityID)).
					WillRespondWith(200, func(b *consumer.V4ResponseBuilder) {
						b.JSONBody(map[string]interface{}{
							"metadata": map[string]interface{}{
								"id":                 matchers.UUID(),
								"name":               matchers.String("kubernetes-cluster-test"),
								"organizationId":     matchers.UUID(),
								"projectId":          matchers.UUID(),
								"creationTime":       matchers.Timestamp(),
								"provisioningStatus": matchers.String("provisioned"),
							},
							"spec": map[string]interface{}{
								"regionId": matchers.UUID(),
								"openstack": map[string]interface{}{
									"cloudConfig":   matchers.String("ZW5jb2RlZC1jbG91ZC1jb25maWc="),
									"cloud":         matchers.String("openstack"),
									"serverGroupId": matchers.String("server-group-id"),
									"sshKeyName":    matchers.String("ssh-key-name"),
								},
							},
						})
					})

				test := func(config consumer.MockServerConfig) error {
					regionClient, err := createRegionClient(config)
					if err != nil {
						return fmt.Errorf("creating region client: %w", err)
					}

					resp, err := regionClient.GetApiV1OrganizationsOrganizationIDProjectsProjectIDIdentitiesIdentityIDWithResponse(
						ctx, organizationID, projectID, identityID)
					if err != nil {
						return fmt.Errorf("getting identity: %w", err)
					}

					Expect(resp.StatusCode()).To(Equal(200))
					Expect(resp.JSON200).NotTo(BeNil())

					return nil
				}

				Expect(pact.ExecuteTest(testingT, test)).To(Succeed())
			})
		})

		Context("when deleting an identity", func() {
			It("removes identity and cleans up cloud credentials", func() {
				organizationID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
				projectID := "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"
				identityID := "c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f"

				pact.AddInteraction().
					GivenWithParameter(models.ProviderState{
						Name: "identity exists",
						Parameters: map[string]interface{}{
							"organizationID": organizationID,
							"projectID":      projectID,
							"identityID":     identityID,
						},
					}).
					UponReceiving("a request to delete identity").
					WithRequest("DELETE",
						fmt.Sprintf("/api/v1/organizations/%s/projects/%s/identities/%s",
							organizationID, projectID, identityID)).
					WillRespondWith(202)

				test := func(config consumer.MockServerConfig) error {
					regionClient, err := createRegionClient(config)
					if err != nil {
						return fmt.Errorf("creating region client: %w", err)
					}

					resp, err := regionClient.DeleteApiV1OrganizationsOrganizationIDProjectsProjectIDIdentitiesIdentityIDWithResponse(
						ctx, organizationID, projectID, identityID)
					if err != nil {
						return fmt.Errorf("deleting identity: %w", err)
					}

					Expect(resp.StatusCode()).To(Equal(202))

					return nil
				}

				Expect(pact.ExecuteTest(testingT, test)).To(Succeed())
			})
		})
	})

	Describe("Network Management", func() {
		Context("when creating a physical network", func() {
			It("creates physical VLAN network for bare-metal nodes", func() {
				organizationID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
				projectID := "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"
				identityID := "c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f"

				pact.AddInteraction().
					GivenWithParameter(models.ProviderState{
						Name: "identity exists with physical network support",
						Parameters: map[string]interface{}{
							"organizationID": organizationID,
							"projectID":      projectID,
							"identityID":     identityID,
						},
					}).
					UponReceiving("a request to create physical network for kubernetes cluster").
					WithRequest("POST", fmt.Sprintf("/api/v1/organizations/%s/projects/%s/identities/%s/networks",
						organizationID, projectID, identityID), func(b *consumer.V4RequestBuilder) {
						b.JSONBody(map[string]interface{}{
							"metadata": map[string]interface{}{
								"name":        matchers.String("kubernetes-cluster-test"),
								"description": matchers.String("Physical network for cluster"),
							},
							"spec": map[string]interface{}{
								"prefix":         matchers.String("192.168.0.0/24"),
								"dnsNameservers": matchers.EachLike(matchers.String("8.8.8.8"), 1),
							},
						})
					}).
					WillRespondWith(201, func(b *consumer.V4ResponseBuilder) {
						b.JSONBody(map[string]interface{}{
							"metadata": map[string]interface{}{
								"id":                 matchers.UUID(),
								"name":               matchers.String("kubernetes-cluster-test"),
								"organizationId":     matchers.UUID(),
								"projectId":          matchers.UUID(),
								"creationTime":       matchers.Timestamp(),
								"provisioningStatus": matchers.String("unknown"),
							},
							"spec": map[string]interface{}{
								"prefix":         matchers.String("192.168.0.0/24"),
								"dnsNameservers": matchers.EachLike(matchers.String("8.8.8.8"), 1),
							},
						})
					})

				test := func(config consumer.MockServerConfig) error {
					regionClient, err := createRegionClient(config)
					if err != nil {
						return fmt.Errorf("creating region client: %w", err)
					}

					networkReq := regionapi.NetworkWrite{
						Metadata: coreclient.ResourceWriteMetadata{
							Name:        "kubernetes-cluster-test",
							Description: func() *string { s := "Physical network for cluster"; return &s }(),
						},
						Spec: &regionapi.NetworkWriteSpec{
							Prefix:         "192.168.0.0/24",
							DnsNameservers: []string{"8.8.8.8"},
						},
					}

					resp, err := regionClient.PostApiV1OrganizationsOrganizationIDProjectsProjectIDIdentitiesIdentityIDNetworksWithResponse(
						ctx, organizationID, projectID, identityID, networkReq)
					if err != nil {
						return fmt.Errorf("creating network: %w", err)
					}

					Expect(resp.StatusCode()).To(Equal(201))
					Expect(resp.JSON201).NotTo(BeNil())

					return nil
				}

				Expect(pact.ExecuteTest(testingT, test)).To(Succeed())
			})
		})

		Context("when getting a network", func() {
			It("retrieves network with OpenStack configuration", func() {
				organizationID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
				projectID := "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"
				identityID := "c3d4e5f6-a7b8-4c9d-0e1f-2a3b4c5d6e7f"
				networkID := "d4e5f6a7-b8c9-4d0e-1f2a-3b4c5d6e7f8a"

				pact.AddInteraction().
					GivenWithParameter(models.ProviderState{
						Name: "network is provisioned",
						Parameters: map[string]interface{}{
							"organizationID": organizationID,
							"projectID":      projectID,
							"identityID":     identityID,
							"networkID":      networkID,
							"regionType":     "openstack",
						},
					}).
					UponReceiving("a request to get provisioned network").
					WithRequest("GET", fmt.Sprintf("/api/v1/organizations/%s/projects/%s/identities/%s/networks/%s",
						organizationID, projectID, identityID, networkID)).
					WillRespondWith(200, func(b *consumer.V4ResponseBuilder) {
						b.JSONBody(map[string]interface{}{
							"metadata": map[string]interface{}{
								"id":                 matchers.UUID(),
								"name":               matchers.UUID(), // Resource ID is used as name
								"organizationId":     matchers.UUID(),
								"projectId":          matchers.UUID(),
								"creationTime":       matchers.Timestamp(),
								"provisioningStatus": matchers.String("provisioned"),
							},
							"spec": map[string]interface{}{
								"prefix":         matchers.String("192.168.0.0/24"),
								"dnsNameservers": matchers.EachLike(matchers.String("8.8.8.8"), 1),
								"openstack": map[string]interface{}{
									"networkId": matchers.String("openstack-network-id"),
									"subnetId":  matchers.String("openstack-subnet-id"),
								},
							},
						})
					})

				test := func(config consumer.MockServerConfig) error {
					regionClient, err := createRegionClient(config)
					if err != nil {
						return fmt.Errorf("creating region client: %w", err)
					}

					resp, err := regionClient.GetApiV1OrganizationsOrganizationIDProjectsProjectIDIdentitiesIdentityIDNetworksNetworkIDWithResponse(
						ctx, organizationID, projectID, identityID, networkID)
					if err != nil {
						return fmt.Errorf("getting network: %w", err)
					}

					Expect(resp.StatusCode()).To(Equal(200))
					Expect(resp.JSON200).NotTo(BeNil())

					return nil
				}

				Expect(pact.ExecuteTest(testingT, test)).To(Succeed())
			})
		})

		Context("when listing external networks", func() {
			It("retrieves available external networks for cluster provisioning", func() {
				organizationID := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
				regionID := "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"

				pact.AddInteraction().
					GivenWithParameter(models.ProviderState{
						Name: "region has external networks",
						Parameters: map[string]interface{}{
							"organizationID": organizationID,
							"regionID":       regionID,
							"regionType":     "openstack",
						},
					}).
					UponReceiving("a request to list external networks").
					WithRequest("GET", fmt.Sprintf("/api/v1/organizations/%s/regions/%s/externalnetworks",
						organizationID, regionID)).
					WillRespondWith(200, func(b *consumer.V4ResponseBuilder) {
						b.JSONBody(matchers.EachLike(map[string]interface{}{
							"id": matchers.UUID(),
						}, 1))
					})

				test := func(config consumer.MockServerConfig) error {
					regionClient, err := createRegionClient(config)
					if err != nil {
						return fmt.Errorf("creating region client: %w", err)
					}

					resp, err := regionClient.GetApiV1OrganizationsOrganizationIDRegionsRegionIDExternalnetworksWithResponse(
						ctx, organizationID, regionID)
					if err != nil {
						return fmt.Errorf("listing external networks: %w", err)
					}

					Expect(resp.StatusCode()).To(Equal(200))
					Expect(resp.JSON200).NotTo(BeNil())
					Expect(*resp.JSON200).NotTo(BeEmpty())

					return nil
				}

				Expect(pact.ExecuteTest(testingT, test)).To(Succeed())
			})
		})
	})
})
