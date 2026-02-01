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

package cilium

import (
	"context"
	"fmt"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	coreclient "github.com/unikorn-cloud/core/pkg/client"
	coreerrors "github.com/unikorn-cloud/core/pkg/errors"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	"github.com/unikorn-cloud/core/pkg/provisioners/util"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
)

// New returns a new initialized provisioner object.
func New(getApplication application.GetterFunc) *application.Provisioner {
	provisioner := &Provisioner{}

	return application.New(getApplication).WithGenerator(provisioner)
}

type Provisioner struct{}

// Ensure the Provisioner interface is implemented.
var _ application.ValuesGenerator = &Provisioner{}

func (p *Provisioner) Values(ctx context.Context, _ unikornv1core.SemanticVersion) (any, error) {
	//nolint:forcetypeassert
	cluster := application.FromContext(ctx).(*unikornv1.KubernetesCluster)

	// Scale to zero support.
	operatorValues := map[string]any{
		"nodeSelector": util.ControlPlaneNodeSelector(),
		"tolerations":  append(util.ControlPlaneInitTolerations(), util.ControlPlaneTolerations()...),
	}

	// If the cluster CP has one node, then this will fail to deploy
	// as cilium has 2 as the default, we need to work some magic here.
	if cluster.Spec.ControlPlane.Replicas == 1 {
		operatorValues["replicas"] = cluster.Spec.ControlPlane.Replicas
	}
	// We run in sans-kube-proxy mode, as it's faster and doesn't involve
	// iptables ;-), for that we need to specify the Kubernetes API endpoint
	// as per https://docs.cilium.io/en/stable/network/kubernetes/kubeproxy-free.
	// This information is propagated as part of the cluster metadata in the
	// context.  Raise an error if not set, as this should only be used in
	// the context of a remote cluster.
	clusterContext, err := coreclient.ClusterFromContext(ctx)
	if err != nil {
		return nil, err
	}

	if clusterContext.Host == "" || clusterContext.Port == "" {
		return nil, fmt.Errorf("%w: missing cluster host:port", coreerrors.ErrInvalidContext)
	}

	values := map[string]any{
		"operator":             operatorValues,
		"kubeProxyReplacement": "true",
		"k8sServiceHost":       clusterContext.Host,
		"k8sServicePort":       clusterContext.Port,
		"hubble": map[string]any{
			"relay": map[string]any{
				"nodeSelector": util.ControlPlaneNodeSelector(),
				"tolerations":  util.ControlPlaneTolerations(),
			},
			"ui": map[string]any{
				"nodeSelector": util.ControlPlaneNodeSelector(),
				"tolerations":  util.ControlPlaneTolerations(),
			},
		},
		"ipam": map[string]any{
			"operator": map[string]any{
				"clusterPoolIPv4PodCIDRList": []any{
					cluster.Spec.Network.PodNetwork.String(),
				},
			},
		},
	}

	return values, nil
}
