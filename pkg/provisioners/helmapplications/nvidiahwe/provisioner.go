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

package nvidiahwe

import (
	"context"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	"github.com/unikorn-cloud/core/pkg/provisioners/util"
)

// New returns a new initialized provisioner object.
func New(getApplication application.GetterFunc) *application.Provisioner {
	p := &Provisioner{}

	return application.New(getApplication).WithGenerator(p)
}

type Provisioner struct{}

// Ensure the Provisioner interface is implemented.
var _ application.ValuesGenerator = &Provisioner{}

// Values implements the application.ValuesGenerator interface.
func (p *Provisioner) Values(ctx context.Context, version unikornv1core.SemanticVersion) (any, error) {
	values := map[string]any{
		// The NFD garbage collector is a Deployment and has no tolerations in the
		// chart defaults, so it needs explicit control-plane tolerations to allow
		// it to schedule before worker nodes are available.
		"node-feature-discovery": map[string]any{
			"gc": map[string]any{
				"tolerations": util.ControlPlaneTolerations(),
			},
		},
		// The GPU operator Deployment itself needs to be pinned to control-plane
		// nodes and tolerate their taints, consistent with the standalone
		// nvidia-gpu-operator provisioner.  Daemonsets already use operator:Exists
		// tolerations in the chart defaults so those require no override.
		"gpu-operator": map[string]any{
			"operator": map[string]any{
				"affinity": map[string]any{
					"nodeAffinity": map[string]any{
						"preferredDuringSchedulingIgnoredDuringExecution": nil,
						"requiredDuringSchedulingIgnoredDuringExecution": map[string]any{
							"nodeSelectorTerms": []any{
								map[string]any{
									"matchExpressions": []any{
										map[string]any{
											"key":      "node-role.kubernetes.io/control-plane",
											"operator": "Exists",
										},
									},
								},
							},
						},
					},
				},
				"tolerations": util.ControlPlaneTolerations(),
			},
		},
	}

	return values, nil
}
