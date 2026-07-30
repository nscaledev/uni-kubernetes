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

package nvidiahwe_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/nvidiahwe"
)

func TestValuesDriverRDMA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		enabled  bool
		expected bool
	}{
		{
			name:     "disabled",
			expected: false,
		},
		{
			name:     "enabled",
			enabled:  true,
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			generator := nvidiahwe.NewValuesGenerator(nvidiahwe.ProvisionerOptions{
				RDMAEnabled: test.enabled,
			})

			values, err := generator.Values(t.Context(), unikornv1core.SemanticVersion{})
			require.NoError(t, err)

			valueMap, ok := values.(map[string]any)
			require.True(t, ok)

			gpuOperator, ok := valueMap["gpu-operator"].(map[string]any)
			require.True(t, ok)

			driver, ok := gpuOperator["driver"].(map[string]any)
			require.True(t, ok)

			rdma, ok := driver["rdma"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, test.expected, rdma["enabled"])
		})
	}
}
