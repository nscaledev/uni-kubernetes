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

package metricsserver

import (
	"context"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	"github.com/unikorn-cloud/core/pkg/provisioners/util"
)

type Provisioner struct{}

// Ensure the Provisioner interface is implemented.
var _ application.ValuesGenerator = &Provisioner{}

// New returns a new initialized provisioner object.
func New(getApplication application.GetterFunc) *application.Provisioner {
	p := &Provisioner{}

	return application.New(getApplication).WithGenerator(p)
}

// Generate implements the application.Generator interface.
// This forces the server onto the control plane rather than take up a
// worker node (and thus incur the ire of users).
func (p *Provisioner) Values(ctx context.Context, version unikornv1core.SemanticVersion) (any, error) {
	values := map[string]any{
		"tolerations":  util.ControlPlaneTolerations(),
		"nodeSelector": util.ControlPlaneNodeSelector(),
	}

	return values, nil
}
