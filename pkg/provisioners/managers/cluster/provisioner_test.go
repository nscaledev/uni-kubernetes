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

package cluster_test

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/managers/cluster"
)

const (
	rdmaRegionID  = "7843c0aa-a314-4b12-a6a0-18be0ba47222"
	otherRegionID = "ce550082-aa4f-49f1-95c1-fec9978afceb"
)

func TestNvidiaRDMAEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		regionID string
		expected bool
	}{
		{
			name:     "empty allowlist",
			regionID: rdmaRegionID,
			expected: false,
		},
		{
			name:     "matching region",
			args:     []string{"--nvidia-rdma-enabled-region-ids=" + rdmaRegionID + "," + otherRegionID},
			regionID: otherRegionID,
			expected: true,
		},
		{
			name:     "non-matching region",
			args:     []string{"--nvidia-rdma-enabled-region-ids=" + rdmaRegionID},
			regionID: otherRegionID,
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			options := &cluster.Options{}
			flags := pflag.NewFlagSet(t.Name(), pflag.ContinueOnError)
			options.AddFlags(flags)
			require.NoError(t, flags.Parse(test.args))

			require.Equal(t, test.expected, cluster.NvidiaRDMAEnabled(options, test.regionID))
		})
	}
}
