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

package cluster_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	unikorncorev1 "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/core/pkg/constants"
	servererrors "github.com/unikorn-cloud/core/pkg/server/errors"
	"github.com/unikorn-cloud/identity/pkg/handler/common/fixtures"
	identityids "github.com/unikorn-cloud/identity/pkg/ids"
	identityapi "github.com/unikorn-cloud/identity/pkg/openapi"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	kubernetesids "github.com/unikorn-cloud/kubernetes/pkg/ids"
	"github.com/unikorn-cloud/kubernetes/pkg/internal/applicationbundle"
	"github.com/unikorn-cloud/kubernetes/pkg/openapi"
	"github.com/unikorn-cloud/kubernetes/pkg/server/handler/cluster"
	"github.com/unikorn-cloud/kubernetes/pkg/server/handler/region"
	regionids "github.com/unikorn-cloud/region/pkg/ids"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	otherRegionID   = "0f2a58ab-5e7c-4a39-a11a-5a8d4c5ac0b1"
	storedClusterID = "5d2f2c4a-6a6c-4f3e-9d0e-6c1f2b9d7a10"
	allocationID    = "b0a5e4c2-3d52-4a62-a1a4-2f6b6a3f1c55"
)

// fakeAPI stands in for a remote service.  It records every request it gets
// and answers from route, or with 404 when route has no answer.
type fakeAPI struct {
	route func(r *http.Request) (any, bool)

	mu       sync.Mutex
	requests []string
}

func (f *fakeAPI) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	f.mu.Unlock()

	status := http.StatusOK

	body, ok := f.route(r)
	if !ok {
		status = http.StatusNotFound
		body = map[string]any{}
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
		Request:    r,
	}, nil
}

func (f *fakeAPI) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.requests...)
}

// regionRoute serves the same flavors and images for every region, so a call
// for any region ID succeeds and is only visible through the request record.
func regionRoute(r *http.Request) (any, bool) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/flavors"):
		return flavorFixtures(), true
	case strings.HasSuffix(r.URL.Path, "/images"):
		return imageFixtures(), true
	}

	return nil, false
}

func identityRoute(r *http.Request) (any, bool) {
	if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/allocations/") {
		return identityapi.AllocationRead{}, true
	}

	return nil, false
}

// updateHarness wires a cluster client to an in memory Kubernetes API that holds
// one existing cluster, and to fake region and identity services.
type updateHarness struct {
	client   client.Client
	clusters *cluster.Client
	region   *fakeAPI
	identity *fakeAPI
}

func newUpdateHarness(t *testing.T, storedRegionID string) *updateHarness {
	t.Helper()

	scheme := runtime.NewScheme()

	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, unikornv1.AddToScheme(scheme))

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultNamespace,
			Labels: map[string]string{
				constants.KindLabel:         constants.KindLabelValueProject,
				constants.OrganizationLabel: organizationID,
				constants.ProjectLabel:      projectID,
			},
		},
	}

	clusterManager := &unikornv1.ClusterManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clustermanagerID,
			Namespace: defaultNamespace,
		},
	}

	existing := existingClusterFixture(t, kubernetesVersion2)
	existing.Name = storedClusterID
	existing.Namespace = defaultNamespace
	existing.Annotations = map[string]string{constants.AllocationAnnotation: allocationID}
	existing.Spec.RegionID = storedRegionID

	options := newGeneratorOptions()
	existing.Spec.Network = unikornv1.KubernetesClusterNetworkSpec{
		NetworkGeneric: unikorncorev1.NetworkGeneric{
			NodeNetwork: unikorncorev1.IPv4Prefix{IPNet: options.NodeNetwork},
		},
		ServiceNetwork: unikorncorev1.IPv4Prefix{IPNet: options.ServiceNetwork},
		PodNetwork:     unikorncorev1.IPv4Prefix{IPNet: options.PodNetwork},
	}

	// The allocation reference is built from the REST mapping of the cluster kind.
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(unikornv1.SchemeGroupVersion.WithKind("KubernetesCluster"), meta.RESTScopeNamespace)

	k8s := fake.NewClientBuilder().WithScheme(scheme).WithRESTMapper(mapper).WithObjects(namespace, clusterManager, existing).Build()

	regionService := &fakeAPI{route: regionRoute}
	identityService := &fakeAPI{route: identityRoute}

	regionClient, err := regionapi.NewClientWithResponses("http://region.example", regionapi.WithHTTPClient(&http.Client{Transport: regionService}))
	require.NoError(t, err)

	identityClient, err := identityapi.NewClientWithResponses("http://identity.example", identityapi.WithHTTPClient(&http.Client{Transport: identityService}))
	require.NoError(t, err)

	return &updateHarness{
		client:   k8s,
		clusters: cluster.NewClient(k8s, options, identityClient, region.New(regionClient)),
		region:   regionService,
		identity: identityService,
	}
}

func (h *updateHarness) update(t *testing.T, request *openapi.KubernetesClusterWrite) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	ctx = fixtures.HandlerContextFixture(ctx, fixtures.WithOrganization|fixtures.WithProject)

	appclient := applicationbundle.NewClient(h.client, defaultNamespace)

	return h.clusters.Update(ctx, appclient, identityids.MustParseOrganizationID(organizationID), identityids.MustParseProjectID(projectID), kubernetesids.MustParseKubernetesClusterID(storedClusterID), request)
}

func (h *updateHarness) stored(t *testing.T) *unikornv1.KubernetesCluster {
	t.Helper()

	result := &unikornv1.KubernetesCluster{}

	require.NoError(t, h.client.Get(t.Context(), client.ObjectKey{Namespace: defaultNamespace, Name: storedClusterID}, result))

	return result
}

// storedBytes returns the stored cluster as serialized, so a comparison also
// catches a change to metadata such as the resource version.
func (h *updateHarness) storedBytes(t *testing.T) []byte {
	t.Helper()

	data, err := json.Marshal(h.stored(t))
	require.NoError(t, err)

	return data
}

// TestClusterUpdateRefusesRegionChange checks that the region of a cluster cannot
// change on update, and that the refusal happens before any call to the region
// service and before any write.
func TestClusterUpdateRefusesRegionChange(t *testing.T) {
	t.Parallel()

	h := newUpdateHarness(t, regionID)
	before := h.storedBytes(t)

	request := clusterRequestFixture(kubernetesVersion2)
	request.Spec.RegionId = regionids.MustParseRegionID(otherRegionID)

	err := h.update(t, request)
	require.Error(t, err)
	require.True(t, servererrors.IsBadRequest(err), "expected a bad request error, got: %v", err)

	require.Equal(t, before, h.storedBytes(t))
	require.Empty(t, h.region.Requests())
	require.Empty(t, h.identity.Requests())
}

// TestClusterUpdateAcceptsSameRegion checks that an update that names the region
// the cluster already has still works, whatever textual form the stored ID has.
// It pairs with the refusal test, as it shows that the fake region service does
// record calls, so an empty record there means no call was made.
func TestClusterUpdateAcceptsSameRegion(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"stored in canonical form": regionID,
		"stored in upper case":     strings.ToUpper(regionID),
	}

	for name, storedRegionID := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newUpdateHarness(t, storedRegionID)

			request := clusterRequestFixture(kubernetesVersion2)
			request.Spec.WorkloadPools[0].Machine.Replicas = ptr.To(2)

			require.NoError(t, h.update(t, request))

			stored := h.stored(t)
			require.Equal(t, regionID, strings.ToLower(stored.Spec.RegionID))
			require.Len(t, stored.Spec.WorkloadPools.Pools, 1)
			require.Equal(t, 2, stored.Spec.WorkloadPools.Pools[0].Replicas)

			require.NotEmpty(t, h.region.Requests())
		})
	}
}

// TestClusterUpdateRepairsStoredRegionThatIsNotAUUID checks that an update works
// when the stored region is not a UUID.  Such a value cannot match the region of
// the request, so the update writes the region of the request over it.  The
// record of the region service shows that the update ran.
func TestClusterUpdateRepairsStoredRegionThatIsNotAUUID(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"stored as text":  "not-a-uuid",
		"stored as empty": "",
	}

	for name, storedRegionID := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newUpdateHarness(t, storedRegionID)

			request := clusterRequestFixture(kubernetesVersion2)
			request.Spec.WorkloadPools[0].Machine.Replicas = ptr.To(2)

			require.NoError(t, h.update(t, request))

			stored := h.stored(t)
			require.Equal(t, regionID, stored.Spec.RegionID)
			require.Len(t, stored.Spec.WorkloadPools.Pools, 1)
			require.Equal(t, 2, stored.Spec.WorkloadPools.Pools[0].Replicas)

			require.NotEmpty(t, h.region.Requests())
		})
	}
}
