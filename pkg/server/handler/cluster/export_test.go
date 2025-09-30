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

package cluster

import (
	"context"

	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/openapi"
)

var NewGenerator = newGenerator

func WithExisting(g *generator, existing *unikornv1.KubernetesCluster) *generator {
	return g.withExisting(existing)
}

func GenerateApplicationBundleName(ctx context.Context, g *generator, appclient appBundleLister, in *openapi.KubernetesClusterWrite) (string, error) {
	return g.generateApplicationBundleName(ctx, appclient, in)
}

func Generate(ctx context.Context, g *generator, appclient appBundleLister, clusterManager *unikornv1.ClusterManager, request *openapi.KubernetesClusterWrite) (*unikornv1.KubernetesCluster, error) {
	return g.generate(ctx, appclient, clusterManager, request)
}
