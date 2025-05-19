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

package virtualcluster

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/pflag"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// On home broadband it'll take about 90s to pull down images, plus any
	// readiness gates we put in the way.  If images are cached then 20s.
	//nolint:gochecknoglobals
	durationMetric = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "unikorn_virtual_kubernetes_provision_duration",
		Help: "Time taken for virtual cluster to provision",
		Buckets: []float64{
			1, 5, 10, 15, 20, 30, 45, 60, 90, 120,
		},
	})
)

var (
	errNoVKCInContext = errors.New("no VirtualKubernetesCluster in context")
)

//nolint:gochecknoinits
func init() {
	metrics.Registry.MustRegister(durationMetric)
}

type ProvisionerOptions struct {
	Domain            string
	NodeSelectorLabel string
	// if true, then instead of making a label `foo: <clusterName>` for the selector,
	// make `foo/<clusterName>: ""` (assuming the NodeSelectorLabel is `foo/`)
	NodeSelectorLabelIsPrefix bool
}

func (opts *ProvisionerOptions) AddFlags(f *pflag.FlagSet) {
	f.StringVar(&opts.Domain, "virtual-kubernetes-cluster-domain", "virtual-kubernetes.example.com", "DNS domain for vclusters to be hosts of.")
	f.StringVar(&opts.NodeSelectorLabel, "node-selector-label", "", "Label to use for vCluster node selectors (will be given the value of the vcluster name, in the selector).")
	f.BoolVar(&opts.NodeSelectorLabelIsPrefix, "node-selector-label-is-prefix", false, `If set, the node selector label will be the vcluster name appended to --node-selector-label after a '/', and the value an empty string`)
}

// NodeSelector creates a `MatchLabels`-style map for supplying to the vcluster chart, based
// on the options given. This is used to restrict the nodes that will be available to the vcluster.
// `vclusterName` is any value that identifies the vcluster in question.
func (opts *ProvisionerOptions) NodeSelector(vclusterName string) map[string]string {
	var selector map[string]string
	if nodeSelectorLabel := opts.NodeSelectorLabel; nodeSelectorLabel != "" {
		selector = map[string]string{}
		if opts.NodeSelectorLabelIsPrefix {
			selector[nodeSelectorLabel+"/"+vclusterName] = ""
		} else {
			selector[nodeSelectorLabel] = vclusterName
		}
	}

	return selector
}

type Provisioner struct {
	Options ProvisionerOptions
}

// New returns a new initialized provisioner object.
func New(getApplication application.GetterFunc, options ProvisionerOptions) *application.Provisioner {
	p := &Provisioner{
		Options: options,
	}

	return application.New(getApplication).WithGenerator(p)
}

func ReleaseName(cluster *unikornv1.VirtualKubernetesCluster) string {
	// This must be no longer than 53 characters and unique across all control
	// planes to avoid OpenStack network aliasing.
	sum := sha256.Sum256([]byte(cluster.Name))

	hash := fmt.Sprintf("%x", sum)

	return "vcluster-" + hash[:8]
}

func (*Provisioner) ReleaseName(ctx context.Context) string {
	//nolint:forcetypeassert
	cluster := application.FromContext(ctx).(*unikornv1.VirtualKubernetesCluster)

	return ReleaseName(cluster)
}

// Values implements the application.ValuesGenerator interface.
func (p *Provisioner) Values(ctx context.Context, version unikornv1core.SemanticVersion) (any, error) {
	// TODO: syncer... loking forward to a world where things are dynamically scheduled
	// and the cost is "what you use", we'll need to worry about billing, so it may
	// be prudent to add organization, project and cluster labels to pods.
	// We use SNI to demutiplex at the ingress to the correct vcluster instance.
	vkc, ok := application.FromContext(ctx).(*unikornv1.VirtualKubernetesCluster)
	if !ok {
		return nil, errNoVKCInContext
	}

	releaseName := p.ReleaseName(ctx)
	hostname := releaseName + "." + p.Options.Domain

	// Allow users to actually hit the cluster.
	ingress := map[string]any{
		"host": hostname,
		"spec": map[string]any{
			"tls": []any{
				map[string]any{
					"hosts": []any{
						hostname,
					},
				},
			},
		},
		"annotations": map[string]any{
			"external-dns.alpha.kubernetes.io/hostname": hostname,
		},
	}

	controlPlane := map[string]any{
		"ingress": ingress,
	}

	sync := map[string]any{}

	// Supply a node selector to the vcluster if the options say to use one. The release name is
	// used as the vcluster name.
	if selector := p.Options.NodeSelector(releaseName); selector != nil {
		sync = map[string]any{
			"fromHost": map[string]any{
				"nodes": map[string]any{
					"selector": selector,
				},
			},
		}
	}

	// Block all network traffic between vclusters and the underlying system,
	// with the exception of egress traffic to the internet.
	// TODO: we probably want to enable the metric-server integration, and that
	// requires a separate network policy adding.
	// apiVersion: networking.k8s.io/v1
	//
	// kind: NetworkPolicy
	// metadata:
	//   name: unikorn-metrics-server
	// spec:
	//   podSelector:
	//     matchLabels:
	//       release: {{ .hostname }}
	//   egress:
	//     - ports:
	//       - port: 443
	//     - to:
	//         - podSelector: {}
	//         - namespaceSelector:
	//             matchLabels:
	//               kubernetes.io/metadata.name: kube-system
	//           podSelector:
	//             matchLabels:
	//               k8s-app: metrics-server
	//   policyTypes:
	//     - Egress

	kubeConfig := map[string]any{
		"server": "https://" + hostname,
	}

	values := map[string]any{
		"vcluster": map[string]any{ // values for the `vcluster` subchart
			"controlPlane":     controlPlane,
			"exportKubeConfig": kubeConfig,
			"sync":             sync,
		},
		"workloadPools": workloadPoolsAsValues(vkc),
	}

	return values, nil
}

func workloadPoolsAsValues(vkc *unikornv1.VirtualKubernetesCluster) []any {
	pools := make([]any, len(vkc.Spec.WorkloadPools))
	for i, pool := range vkc.Spec.WorkloadPools {
		pools[i] = map[string]any{
			"name":     pool.Name,
			"replicas": pool.Replicas,
			"flavorId": pool.FlavorID,
		}
	}

	return pools
}
