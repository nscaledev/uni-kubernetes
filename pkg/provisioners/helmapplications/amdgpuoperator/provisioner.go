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

package amdgpuoperator

import (
	"context"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	"github.com/unikorn-cloud/core/pkg/provisioners/util"
)

type Provisioner struct{}

// New returns a new initialized provisioner object.
func New(getApplication application.GetterFunc) *application.Provisioner {
	p := &Provisioner{}

	return application.New(getApplication).WithGenerator(p)
}

func (p *Provisioner) Values(ctx context.Context, version unikornv1core.SemanticVersion) (any, error) {
	// Inject tolerations to allow cluster provisoning, especially useful for
	// baremetal where the nodes take ages to come into existence and Argo decides
	// it's going to give up trying to sync.
	values := map[string]any{
		"node-feature-discovery": map[string]any{
			"gc": map[string]any{
				"tolerations": util.ControlPlaneTolerations(),
			},
		},
	}

	return values, nil
}
