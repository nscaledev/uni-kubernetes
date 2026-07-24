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

package cilium_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	coreclient "github.com/unikorn-cloud/core/pkg/client"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/cilium"
)

// TestValuesConfineSocketLBToHostNamespace verifies kube-proxy replacement
// leaves pod service traffic visible to service-mesh sidecars.
func TestValuesConfineSocketLBToHostNamespace(t *testing.T) {
	t.Parallel()

	_, podNetwork, err := net.ParseCIDR("10.0.0.0/16")
	require.NoError(t, err)

	cluster := &unikornv1.KubernetesCluster{
		Spec: unikornv1.KubernetesClusterSpec{
			ControlPlane: unikornv1core.MachineGeneric{
				Replicas: 3,
			},
			Network: unikornv1.KubernetesClusterNetworkSpec{
				PodNetwork: unikornv1core.IPv4Prefix{
					IPNet: *podNetwork,
				},
			},
		},
	}

	ctx := application.NewContext(t.Context(), cluster)
	ctx = coreclient.NewContextWithCluster(ctx, &coreclient.ClusterContext{
		Host: "api.example.com",
		Port: "6443",
	})

	generated, err := (&cilium.Provisioner{}).Values(ctx, unikornv1core.SemanticVersion{})
	require.NoError(t, err)

	values, ok := generated.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "true", values["kubeProxyReplacement"])

	socketLB, ok := values["socketLB"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, socketLB["hostNamespaceOnly"])
}
