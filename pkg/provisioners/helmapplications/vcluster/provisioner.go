/*
Copyright 2022-2024 EscherCloud.
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

package vcluster

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/unikorn-cloud/core/pkg/provisioners/application"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// On home broadband it'll take about 90s to pull down images, plus any
	// readiness gates we put in the way.  If images are cached then 20s.
	//nolint:gochecknoglobals
	durationMetric = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "unikorn_vcluster_provision_duration",
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

type Provisioner struct{}

func (*Provisioner) ReleaseName(ctx context.Context) string {
	return releaseName(application.FromContext(ctx).GetName())
}

// New returns a new initialized provisioner object.
func New(getApplication application.GetterFunc, name string) *application.Provisioner {
	return application.New(getApplication).WithGenerator(&Provisioner{})
}
