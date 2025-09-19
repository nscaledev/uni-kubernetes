/*
Copyright 2024-2025 the Unikorn Authors.

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

package observabilityagent

import (
	"context"
	"fmt"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	coreerrors "github.com/unikorn-cloud/core/pkg/errors"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
)

// Provisioner configures the observability agent chart.
type Provisioner struct{}

// Ensure interface conformance.
var _ application.ValuesGenerator = &Provisioner{}

// New returns a configured provisioner.
func New(getApplication application.GetterFunc) *application.Provisioner {
	provisioner := &Provisioner{}

	return application.New(getApplication).WithGenerator(provisioner)
}

// Values implements application.ValuesGenerator.
func (p *Provisioner) Values(ctx context.Context, version unikornv1core.SemanticVersion) (any, error) {
	cluster, ok := application.FromContext(ctx).(*unikornv1.KubernetesCluster)
	if !ok {
		return nil, fmt.Errorf("%w: kubernetes cluster not available in context", coreerrors.ErrInvalidContext)
	}

	values := map[string]any{
		"clusterName": cluster.Name,
		"region":      cluster.Labels[RegionNameLabel],
		"environment": "nks",
	}

	if values["region"] == "" {
		values["region"] = cluster.Spec.RegionID
	}

	return values, nil
}
