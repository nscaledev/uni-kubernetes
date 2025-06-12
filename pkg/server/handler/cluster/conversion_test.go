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

//nolint:testpackage
package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"

	unikorncorev1 "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/internal/applicationbundle"
	"github.com/unikorn-cloud/kubernetes/pkg/openapi"

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
)

const defaultNamespace = "default"
const otherNamespace = "other"

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

	g := newGenerator(c, nil, nil, "", "", "")

	request := &openapi.KubernetesClusterWrite{
		Spec: openapi.KubernetesClusterSpec{},
	}

	appclient := applicationbundle.NewClient(c, defaultNamespace)
	name, err := g.generateApplicationBundleName(ctx, appclient, request)
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

	g := newGenerator(c, nil, nil, "", "", "")

	request := &openapi.KubernetesClusterWrite{
		Spec: openapi.KubernetesClusterSpec{
			ApplicationBundleName: ptr.To(expectedBundleName(versionMiddle)),
		},
	}

	appclient := applicationbundle.NewClient(c, defaultNamespace)
	name, err := g.generateApplicationBundleName(ctx, appclient, request)
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

	g := newGenerator(c, nil, nil, "", "", "").withExisting(existing)

	request := &openapi.KubernetesClusterWrite{
		Spec: openapi.KubernetesClusterSpec{},
	}

	appclient := applicationbundle.NewClient(c, defaultNamespace)
	name, err := g.generateApplicationBundleName(ctx, appclient, request)
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

	g := newGenerator(c, nil, nil, "", "", "").withExisting(existing)

	request := &openapi.KubernetesClusterWrite{
		Spec: openapi.KubernetesClusterSpec{
			ApplicationBundleName: ptr.To(expectedBundleName(versionOldest)),
		},
	}

	appclient := applicationbundle.NewClient(c, defaultNamespace)
	name, err := g.generateApplicationBundleName(ctx, appclient, request)
	require.NoError(t, err)
	require.NotNil(t, name)
	require.Equal(t, expectedBundleName(versionOldest), name)
}
