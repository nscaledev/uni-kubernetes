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

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"

	corev1 "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/core/pkg/constants"
	kubernetesv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func generateManifests(ctx context.Context) ([]string, error) {
	path, err := exec.LookPath("helm")
	if err != nil {
		return nil, err
	}

	output, err := exec.CommandContext(ctx, path, "template", "foo", "charts/kubernetes").CombinedOutput()
	if err != nil {
		return nil, err
	}

	manifests := regexp.MustCompile("---").Split(string(output), -1)

	manifests = slices.DeleteFunc(manifests, func(s string) bool {
		return strings.TrimSpace(s) == ""
	})

	return manifests, nil
}

func parseManifests(manifests []string) ([]*unstructured.Unstructured, error) {
	objects := make([]*unstructured.Unstructured, len(manifests))

	for i, manifest := range manifests {
		u := &unstructured.Unstructured{}

		if err := yaml.Unmarshal([]byte(manifest), u); err != nil {
			return nil, err
		}

		objects[i] = u
	}

	return objects, nil
}

type applicationSet []*corev1.HelmApplication

//nolint:err113
func (a applicationSet) lookup(bundle, application string, reference corev1.ApplicationReference) error {
	for _, app := range a {
		if app.Name != *reference.Name {
			continue
		}

		if _, err := app.GetVersion(reference.Version); err != nil {
			return fmt.Errorf("bundle %s references application version %s:%v that does not exist", bundle, application, reference.Version.ToUnstructured())
		}

		return nil
	}

	return fmt.Errorf("bundle %s references application %s that does not exist", bundle, application)
}

type decodedObjects struct {
	kubernetesClusterApplicationBundles       []*kubernetesv1.KubernetesClusterApplicationBundle
	clusterManagerApplicationBundles          []*kubernetesv1.ClusterManagerApplicationBundle
	virtualKubernetesClusterApplicationBundle []*kubernetesv1.VirtualKubernetesClusterApplicationBundle
	applications                              applicationSet
}

//nolint:cyclop
func decodeObjects(objects []*unstructured.Unstructured) (*decodedObjects, error) {
	kcab := []*kubernetesv1.KubernetesClusterApplicationBundle{}
	cmab := []*kubernetesv1.ClusterManagerApplicationBundle{}
	vkcab := []*kubernetesv1.VirtualKubernetesClusterApplicationBundle{}
	apps := applicationSet{}

	for _, o := range objects {
		gvk := o.GroupVersionKind()

		if gvk.Group != "unikorn-cloud.org" || gvk.Version != "v1alpha1" {
			continue
		}

		switch gvk.Kind {
		case "KubernetesClusterApplicationBundle":
			t := &kubernetesv1.KubernetesClusterApplicationBundle{}

			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(o.Object, t); err != nil {
				return nil, err
			}

			kcab = append(kcab, t)
		case "ClusterManagerApplicationBundle":
			t := &kubernetesv1.ClusterManagerApplicationBundle{}

			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(o.Object, t); err != nil {
				return nil, err
			}

			cmab = append(cmab, t)
		case "VirtualKubernetesClusterApplicationBundle":
			t := &kubernetesv1.VirtualKubernetesClusterApplicationBundle{}

			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(o.Object, t); err != nil {
				return nil, err
			}

			vkcab = append(vkcab, t)
		case "HelmApplication":
			t := &corev1.HelmApplication{}

			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(o.Object, t); err != nil {
				return nil, err
			}

			apps = append(apps, t)
		default:
			continue
		}
	}

	result := &decodedObjects{
		kubernetesClusterApplicationBundles:       kcab,
		clusterManagerApplicationBundles:          cmab,
		virtualKubernetesClusterApplicationBundle: vkcab,
		applications: apps,
	}

	return result, nil
}

func applicationKey(id string, version corev1.SemanticVersion) string {
	return id + "/" + version.Original()
}

type referenceSet map[string]kubernetesv1.ApplicationNamedReference

//nolint:err113
func (r referenceSet) lookup(app *corev1.HelmApplication) error {
	for _, version := range app.Spec.Versions {
		if _, ok := r[applicationKey(app.Name, version.Version)]; !ok {
			return fmt.Errorf("application version %s:%s exists but is not referenced", app.Labels[constants.NameLabel], version.Version.Original())
		}
	}

	return nil
}

func validateApplicationBundle(name string, spec *kubernetesv1.ApplicationBundleSpec, apps applicationSet, references referenceSet) error {
	for _, app := range spec.Applications {
		if err := apps.lookup(name, app.Name, app.Reference); err != nil {
			return err
		}

		references[applicationKey(*app.Reference.Name, app.Reference.Version)] = app
	}

	return nil
}

//nolint:cyclop
func main() {
	ctx := context.Background()

	manifests, err := generateManifests(ctx)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	objects, err := parseManifests(manifests)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	decoded, err := decodeObjects(objects)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	references := referenceSet{}

	// Validate bundles reference application versionsi that actually exist.
	for _, t := range decoded.kubernetesClusterApplicationBundles {
		if err := validateApplicationBundle(t.Name, &t.Spec, decoded.applications, references); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	for _, t := range decoded.clusterManagerApplicationBundles {
		if err := validateApplicationBundle(t.Name, &t.Spec, decoded.applications, references); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	for _, t := range decoded.virtualKubernetesClusterApplicationBundle {
		if err := validateApplicationBundle(t.Name, &t.Spec, decoded.applications, references); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	// Validate applications are cleaned up.
	for _, app := range decoded.applications {
		if err := references.lookup(app); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}
