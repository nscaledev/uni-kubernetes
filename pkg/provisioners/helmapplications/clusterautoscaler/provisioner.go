/*
Copyright 2022-2024 EscherCloud.
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

package clusterautoscaler

import (
	"context"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/clusteropenstack"
)

// Provisioner encapsulates provisioning.
type Provisioner struct{}

// New returns a new initialized provisioner object.
func New(getApplication application.GetterFunc) *application.Provisioner {
	provisoner := &Provisioner{}

	return application.New(getApplication).WithGenerator(provisoner)
}

// Ensure the Provisioner interface is implemented.
var _ application.Paramterizer = &Provisioner{}

func (p *Provisioner) Parameters(ctx context.Context, version unikornv1core.SemanticVersion) (map[string]string, error) {
	//nolint:forcetypeassert
	cluster := application.FromContext(ctx).(*unikornv1.KubernetesCluster)

	parameters := map[string]string{
		"autoDiscovery.clusterName":  clusteropenstack.CAPIClusterName(cluster),
		"clusterAPIKubeconfigSecret": clusteropenstack.KubeconfigSecretName(cluster),
	}

	return parameters, nil
}
