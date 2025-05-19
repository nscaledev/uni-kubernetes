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
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

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
		Help: "Time taken for vcluster to provision",
		Buckets: []float64{
			1, 5, 10, 15, 20, 30, 45, 60, 90, 120,
		},
	})
)

//nolint:gochecknoinits
func init() {
	metrics.Registry.MustRegister(durationMetric)
}

type ProvisionerOptions struct {
	Domain            string
	NodeSelectorLabel string
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
	releaseName := p.ReleaseName(ctx)
	hostname := releaseName + "." + p.Options.Domain

	// Allow users to actually hit the cluster.
	ingress := map[string]any{
		"enabled": true,
		"host":    hostname,
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

	backingStore := map[string]any{
		"etcd": map[string]any{
			"deploy": map[string]any{
				"enabled": true,
				"statefulSet": map[string]any{
					"highAvailability": map[string]int{
						"replicas": 3,
					},
				},
			},
		},
	}

	// Clean up the volume when the cluster is deleted, lest we leak a ton of space.
	statefulSet := map[string]any{
		"persistence": map[string]any{
			"volumeClaim": map[string]any{
				"retentionPolicy": "Delete",
			},
		},
	}

	controlPlane := map[string]any{
		"ingress":      ingress,
		"backingStore": backingStore,
		"statefulSet":  statefulSet,
	}

	syncNodes := map[string]any{
		"enabled":          true,
		"clearImageStatus": true,
	}
	if nodeSelectorLabel := p.Options.NodeSelectorLabel; nodeSelectorLabel != "" {
		syncNodes["selector"] = map[string]any{
			"labels": map[string]string{
				nodeSelectorLabel: releaseName,
			},
		}
	}

	sync := map[string]any{
		"fromHost": map[string]any{
			"nodes": syncNodes,
			"runtimeClasses": map[string]any{
				"enabled": true,
			},
		},
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
	policies := map[string]any{
		"networkPolicy": map[string]any{
			"enabled": true,
		},
	}

	kubeConfig := map[string]any{
		"server": "https://" + hostname,
	}

	values := map[string]any{
		"controlPlane":     controlPlane,
		"policies":         policies,
		"sync":             sync,
		"exportKubeConfig": kubeConfig,
	}

	return values, nil
}
