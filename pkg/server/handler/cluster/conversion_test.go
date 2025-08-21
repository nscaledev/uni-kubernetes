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

package cluster_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	unikorncorev1 "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/core/pkg/constants"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	"github.com/unikorn-cloud/identity/pkg/handler/common/fixtures"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/internal/applicationbundle"
	"github.com/unikorn-cloud/kubernetes/pkg/openapi"
	"github.com/unikorn-cloud/kubernetes/pkg/server/handler/cluster"
	region "github.com/unikorn-cloud/kubernetes/pkg/server/handler/region/mock"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	versionOldest = "v1.0.0"
	versionMiddle = "v1.1.0"
	versionNewest = "v1.1.2"

	defaultNamespace = "default"
	otherNamespace   = "other"

	flavorID1 = "7b1da45b-49bc-466e-a9c4-cea714565879"
	flavorID2 = "0ec9ddd8-9ff0-4094-b9f5-f6bb74094efd"

	imageID1 = "d38d73b4-c2bf-4625-8724-e0afe62a877e"
	imageID2 = "97ade00e-47af-4ea9-b1ea-328cd808ba6c"
	imageID3 = "5f092a7a-31df-45d6-bebe-f0cabc04c563"

	kubernetesVersion1 = "v1.28.0"
	kubernetesVersion2 = "v1.29.0"
	kubernetesVersion3 = "v1.30.0"

	clusterName = "mercury"

	organizationID   = "e10b7219-1c3e-4d17-8b83-67fbd07bbf63"
	projectID        = "412dba0c-9afe-46fb-88f4-c13776185b94"
	regionID         = "a73e9c26-af56-4562-8352-9512e0586f3b"
	clustermanagerID = "cb12a0b7-c1e6-45bc-a907-1dc8581aeb36"
)

func expectedBundleName(version string) string {
	return "bundle-" + version
}

// applicationBundleFixture generates an application bundle object in a terse fashion.
func applicationBundleFixture(t *testing.T, version string, namespace, name string) *unikornv1.KubernetesClusterApplicationBundle {
	t.Helper()

	v, err := semver.NewVersion(version)
	require.NoError(t, err)

	return &unikornv1.KubernetesClusterApplicationBundle{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: unikornv1.ApplicationBundleSpec{
			Version: unikorncorev1.SemanticVersion{
				Version: *v,
			},
		},
	}
}

func flavorFixture(id string, cpus, memory, disk int) *regionapi.Flavor {
	return &regionapi.Flavor{
		Metadata: coreapi.StaticResourceMetadata{
			Id: id,
		},
		Spec: regionapi.FlavorSpec{
			Cpus:   cpus,
			Memory: memory,
			Disk:   disk,
		},
	}
}

func flavorFixtureList(in ...*regionapi.Flavor) regionapi.Flavors {
	out := make(regionapi.Flavors, len(in))

	for i := range in {
		out[i] = *in[i]
	}

	return out
}

func flavorFixtures() regionapi.Flavors {
	return flavorFixtureList(
		flavorFixture(flavorID1, 2, 2, 10),
		flavorFixture(flavorID2, 4, 4, 20),
	)
}

func imageFixture(id, version string) *regionapi.Image {
	return &regionapi.Image{
		Metadata: coreapi.StaticResourceMetadata{
			Id: id,
		},
		Spec: regionapi.ImageSpec{
			SizeGiB: 10,
			SoftwareVersions: &regionapi.SoftwareVersions{
				"kubernetes": version,
			},
		},
	}
}

func imageFixtureList(in ...*regionapi.Image) regionapi.Images {
	out := make(regionapi.Images, len(in))

	for i := range in {
		out[i] = *in[i]
	}

	return out
}

func imageFixtures() regionapi.Images {
	return imageFixtureList(
		imageFixture(imageID1, kubernetesVersion1),
		imageFixture(imageID2, kubernetesVersion2),
		imageFixture(imageID3, kubernetesVersion3),
	)
}

func clusterRequestFixture(version string) *openapi.KubernetesClusterWrite {
	return &openapi.KubernetesClusterWrite{
		Metadata: coreapi.ResourceWriteMetadata{
			Name: clusterName,
		},
		Spec: openapi.KubernetesClusterSpec{
			RegionId:              regionID,
			Version:               version,
			ApplicationBundleName: ptr.To(expectedBundleName(versionOldest)),
			ClusterManagerId:      ptr.To(clustermanagerID),
			WorkloadPools: openapi.KubernetesClusterWorkloadPools{
				{
					Machine: openapi.MachinePool{
						FlavorId: ptr.To(flavorID1),
						Replicas: ptr.To(1),
					},
				},
			},
		},
	}
}

func existingClusterFixture(t *testing.T, version string) *unikornv1.KubernetesCluster {
	t.Helper()

	existingVersion, err := semver.NewVersion(version)
	require.NoError(t, err)

	return &unikornv1.KubernetesCluster{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				constants.NameLabel:         clusterName,
				constants.OrganizationLabel: organizationID,
				constants.ProjectLabel:      projectID,
			},
		},
		Spec: unikornv1.KubernetesClusterSpec{
			RegionID: regionID,
			Version: unikorncorev1.SemanticVersion{
				Version: *existingVersion,
			},
			ControlPlane: unikorncorev1.MachineGeneric{
				Replicas: 3,
				ImageID:  imageID1,
				FlavorID: flavorID1,
			},
			WorkloadPools: unikornv1.KubernetesClusterWorkloadPoolsSpec{
				Pools: []unikornv1.KubernetesWorkloadPoolSpec{
					{
						MachineGeneric: unikorncorev1.MachineGeneric{
							Replicas: 1,
							ImageID:  imageID1,
							FlavorID: flavorID1,
						},
					},
				},
			},
		},
	}
}

func newGeneratorOptions() *cluster.Options {
	return &cluster.Options{
		ControlPlaneCPUsMax:      2,
		ControlPlaneMemoryMaxGiB: 10,
		NodeNetwork: net.IPNet{
			IP:   []byte{192, 168, 0, 0},
			Mask: net.IPMask{255, 255, 0, 0},
		},
		ServiceNetwork: net.IPNet{
			IP:   []byte{172, 16, 0, 0},
			Mask: net.IPMask{255, 240, 0, 0},
		},
		PodNetwork: net.IPNet{
			IP:   []byte{10, 8, 0, 0},
			Mask: net.IPMask{255, 0, 0, 0},
		},
	}
}

// newClient creates a client with some bundle objects of various versions.
func newClient(t *testing.T) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()

	require.NoError(t, unikornv1.AddToScheme(scheme))

	bundles := []client.Object{
		applicationBundleFixture(t, versionMiddle, defaultNamespace, expectedBundleName(versionMiddle)),
		applicationBundleFixture(t, versionNewest, defaultNamespace, expectedBundleName(versionNewest)),
		applicationBundleFixture(t, versionOldest, defaultNamespace, expectedBundleName(versionOldest)),
		// and some false choices
		applicationBundleFixture(t, versionMiddle, otherNamespace, "other-"+expectedBundleName(versionMiddle)),
		applicationBundleFixture(t, versionNewest, otherNamespace, "other-"+expectedBundleName(versionNewest)),
		applicationBundleFixture(t, versionOldest, otherNamespace, "other-"+expectedBundleName(versionOldest)),
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(bundles...).Build()
}

// TestApplicationBundleNameGenerationCreateDefault ensure the latest bundle is selected if none
// is passed via the API.  This is lagacy behaviour.
func TestApplicationBundleNameGenerationCreateDefault(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	c := newClient(t)

	g := cluster.NewGenerator(c, nil, nil, "", "", "")

	request := &openapi.KubernetesClusterWrite{
		Spec: openapi.KubernetesClusterSpec{},
	}

	appclient := applicationbundle.NewClient(c, defaultNamespace)
	name, err := cluster.GenerateApplicationBundleName(ctx, g, appclient, request)
	require.NoError(t, err)
	require.NotNil(t, name)
	require.Equal(t, expectedBundleName(versionNewest), name)
}

// TestApplicationBundleNameGenerationCreateExplicit checks that a bundle is selected explicitly
// overriding and defaults.
func TestApplicationBundleNameGenerationCreateExplicit(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	c := newClient(t)

	g := cluster.NewGenerator(c, nil, nil, "", "", "")

	request := &openapi.KubernetesClusterWrite{
		Spec: openapi.KubernetesClusterSpec{
			ApplicationBundleName: ptr.To(expectedBundleName(versionMiddle)),
		},
	}

	appclient := applicationbundle.NewClient(c, defaultNamespace)
	name, err := cluster.GenerateApplicationBundleName(ctx, g, appclient, request)
	require.NoError(t, err)
	require.NotNil(t, name)
	require.Equal(t, expectedBundleName(versionMiddle), name)
}

// TestApplicationBundleNameGenerationUpdateDefault checks that on update a bundle version
// is preserved and not accidentally upgraded.
func TestApplicationBundleNameGenerationUpdateDefault(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	c := newClient(t)

	existing := &unikornv1.KubernetesCluster{
		Spec: unikornv1.KubernetesClusterSpec{
			ApplicationBundle: expectedBundleName(versionMiddle),
		},
	}

	g := cluster.NewGenerator(c, nil, nil, "", "", "")
	g = cluster.WithExisting(g, existing)

	request := &openapi.KubernetesClusterWrite{
		Spec: openapi.KubernetesClusterSpec{},
	}

	appclient := applicationbundle.NewClient(c, defaultNamespace)
	name, err := cluster.GenerateApplicationBundleName(ctx, g, appclient, request)
	require.NoError(t, err)
	require.NotNil(t, name)
	require.Equal(t, expectedBundleName(versionMiddle), name)
}

// TestApplicationBundleNameGenerationUpdateExplicit checks that a bundle selected at the API
// overrides any existing value.
func TestApplicationBundleNameGenerationUpdateExplicit(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	c := newClient(t)

	existing := &unikornv1.KubernetesCluster{
		Spec: unikornv1.KubernetesClusterSpec{
			ApplicationBundle: expectedBundleName(versionNewest),
		},
	}

	g := cluster.NewGenerator(c, nil, nil, "", "", "")
	g = cluster.WithExisting(g, existing)

	request := &openapi.KubernetesClusterWrite{
		Spec: openapi.KubernetesClusterSpec{
			ApplicationBundleName: ptr.To(expectedBundleName(versionOldest)),
		},
	}

	appclient := applicationbundle.NewClient(c, defaultNamespace)
	name, err := cluster.GenerateApplicationBundleName(ctx, g, appclient, request)
	require.NoError(t, err)
	require.NotNil(t, name)
	require.Equal(t, expectedBundleName(versionOldest), name)
}

// TestClusterGenerate tests that generation correctly sets metadata and picks various defaults.
func TestClusterGenerate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	ctx = fixtures.HandlerContextFixture(ctx, fixtures.WithOrganization)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c := newClient(t)

	appclient := applicationbundle.NewClient(c, defaultNamespace)

	region := region.NewMockClientInterface(ctrl)
	region.EXPECT().Flavors(ctx, organizationID, regionID).AnyTimes().Return(flavorFixtures(), nil)
	region.EXPECT().Images(ctx, organizationID, regionID).AnyTimes().Return(imageFixtures(), nil)

	g := cluster.NewGenerator(c, newGeneratorOptions(), region, defaultNamespace, organizationID, projectID)

	cluster, err := cluster.Generate(ctx, g, appclient, clusterRequestFixture(kubernetesVersion2))
	require.NoError(t, err)

	// Metadata is all correct...
	require.NotNil(t, cluster.Labels)
	require.Equal(t, clusterName, cluster.Labels[constants.NameLabel])
	require.Equal(t, organizationID, cluster.Labels[constants.OrganizationLabel])
	require.Equal(t, projectID, cluster.Labels[constants.ProjectLabel])

	// Globals are all correct...
	require.Equal(t, regionID, cluster.Spec.RegionID)
	require.Equal(t, clustermanagerID, cluster.Spec.ClusterManagerID)

	// Control plane defaults set up correctly...
	require.Equal(t, 3, cluster.Spec.ControlPlane.Replicas)
	require.Equal(t, imageID2, cluster.Spec.ControlPlane.ImageID)
	require.Equal(t, flavorID1, cluster.Spec.ControlPlane.FlavorID)

	// Workload pools defaulted correctly...
	require.Len(t, cluster.Spec.WorkloadPools.Pools, 1)
	require.Equal(t, 1, cluster.Spec.WorkloadPools.Pools[0].Replicas)
	require.Equal(t, imageID2, cluster.Spec.WorkloadPools.Pools[0].ImageID)
	require.Equal(t, flavorID1, cluster.Spec.WorkloadPools.Pools[0].FlavorID)
}

// TestClusterUpgrade tests that in the presence of an existing cluster, where the
// Kubernetes version has been upgraded, the correc image is selected.
func TestClusterUpgrade(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	ctx = fixtures.HandlerContextFixture(ctx, fixtures.WithOrganization)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c := newClient(t)

	appclient := applicationbundle.NewClient(c, defaultNamespace)

	region := region.NewMockClientInterface(ctrl)
	region.EXPECT().Flavors(ctx, organizationID, regionID).AnyTimes().Return(flavorFixtures(), nil)
	region.EXPECT().Images(ctx, organizationID, regionID).AnyTimes().Return(imageFixtures(), nil)

	g := cluster.NewGenerator(c, newGeneratorOptions(), region, defaultNamespace, organizationID, projectID)
	g = cluster.WithExisting(g, existingClusterFixture(t, kubernetesVersion1))

	cluster, err := cluster.Generate(ctx, g, appclient, clusterRequestFixture(kubernetesVersion3))
	require.NoError(t, err)

	// Metadata is all correct...
	require.NotNil(t, cluster.Labels)
	require.Equal(t, clusterName, cluster.Labels[constants.NameLabel])
	require.Equal(t, organizationID, cluster.Labels[constants.OrganizationLabel])
	require.Equal(t, projectID, cluster.Labels[constants.ProjectLabel])

	// Globals are all correct...
	require.Equal(t, regionID, cluster.Spec.RegionID)
	require.Equal(t, clustermanagerID, cluster.Spec.ClusterManagerID)

	// Control plane set up correctly...
	require.Equal(t, 3, cluster.Spec.ControlPlane.Replicas)
	require.Equal(t, imageID3, cluster.Spec.ControlPlane.ImageID)
	require.Equal(t, flavorID1, cluster.Spec.ControlPlane.FlavorID)

	// Workload pools set up correctly...
	require.Len(t, cluster.Spec.WorkloadPools.Pools, 1)
	require.Equal(t, 1, cluster.Spec.WorkloadPools.Pools[0].Replicas)
	require.Equal(t, imageID3, cluster.Spec.WorkloadPools.Pools[0].ImageID)
	require.Equal(t, flavorID1, cluster.Spec.WorkloadPools.Pools[0].FlavorID)
}
