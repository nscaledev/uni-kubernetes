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

package ids

import (
	"github.com/google/uuid"
)

// KubernetesClusterID is a UUID-backed identifier for Kubernetes clusters. It is a
// distinct named type so the compiler prevents accidental interchange with any other
// ID type — a Kubernetes cluster and a virtual Kubernetes cluster are different
// resource kinds (urn:unikorn:kubernetescluster:<uuid> vs
// urn:unikorn:virtualkubernetescluster:<uuid>) even though both are UUIDs.
// UnmarshalText delegates to uuid.UUID, so the oapi-codegen runtime rejects
// non-UUID path parameter values before any handler is reached.
//
//nolint:recvcheck // UnmarshalText must be a pointer receiver; String/MarshalText are value receivers for fmt.Stringer compatibility.
type KubernetesClusterID uuid.UUID

func (v KubernetesClusterID) String() string                { return uuid.UUID(v).String() }
func (v KubernetesClusterID) MarshalText() ([]byte, error)  { return uuid.UUID(v).MarshalText() }
func (v *KubernetesClusterID) UnmarshalText(b []byte) error { return unmarshalUUID((*uuid.UUID)(v), b) }

// VirtualKubernetesClusterID is a UUID-backed identifier for virtual Kubernetes
// clusters. It is a distinct named type so the compiler prevents accidental
// interchange with any other ID type (see KubernetesClusterID). UnmarshalText
// delegates to uuid.UUID, so the oapi-codegen runtime rejects non-UUID path
// parameter values before any handler is reached.
//
//nolint:recvcheck // UnmarshalText must be a pointer receiver; String/MarshalText are value receivers for fmt.Stringer compatibility.
type VirtualKubernetesClusterID uuid.UUID

func (v VirtualKubernetesClusterID) String() string               { return uuid.UUID(v).String() }
func (v VirtualKubernetesClusterID) MarshalText() ([]byte, error) { return uuid.UUID(v).MarshalText() }
func (v *VirtualKubernetesClusterID) UnmarshalText(b []byte) error {
	return unmarshalUUID((*uuid.UUID)(v), b)
}

// ClusterManagerID is a UUID-backed identifier for cluster managers. It is a distinct
// named type so the compiler prevents accidental interchange with any other ID type.
// UnmarshalText delegates to uuid.UUID, so the oapi-codegen runtime rejects
// non-UUID path parameter values before any handler is reached.
//
//nolint:recvcheck // UnmarshalText must be a pointer receiver; String/MarshalText are value receivers for fmt.Stringer compatibility.
type ClusterManagerID uuid.UUID

func (v ClusterManagerID) String() string                { return uuid.UUID(v).String() }
func (v ClusterManagerID) MarshalText() ([]byte, error)  { return uuid.UUID(v).MarshalText() }
func (v *ClusterManagerID) UnmarshalText(b []byte) error { return unmarshalUUID((*uuid.UUID)(v), b) }

// unmarshalUUID is the shared implementation for all UnmarshalText methods.
func unmarshalUUID(dst *uuid.UUID, text []byte) error {
	var id uuid.UUID

	if err := id.UnmarshalText(text); err != nil {
		return err
	}

	*dst = id

	return nil
}

// ParseKubernetesClusterID parses s as a UUID into a KubernetesClusterID, returning
// an error if s is not a valid UUID.
func ParseKubernetesClusterID(s string) (KubernetesClusterID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return KubernetesClusterID{}, err
	}

	return KubernetesClusterID(id), nil
}

// ParseVirtualKubernetesClusterID parses s as a UUID into a VirtualKubernetesClusterID,
// returning an error if s is not a valid UUID.
func ParseVirtualKubernetesClusterID(s string) (VirtualKubernetesClusterID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return VirtualKubernetesClusterID{}, err
	}

	return VirtualKubernetesClusterID(id), nil
}

// ParseClusterManagerID parses s as a UUID into a ClusterManagerID, returning
// an error if s is not a valid UUID.
func ParseClusterManagerID(s string) (ClusterManagerID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return ClusterManagerID{}, err
	}

	return ClusterManagerID(id), nil
}

// MustParseKubernetesClusterID parses s as a UUID into a KubernetesClusterID.
// It panics if s is not a valid UUID, so use it only with compile-time constants
// and in tests.
func MustParseKubernetesClusterID(s string) KubernetesClusterID {
	return KubernetesClusterID(uuid.MustParse(s))
}

// MustParseVirtualKubernetesClusterID parses s as a UUID into a VirtualKubernetesClusterID.
// It panics if s is not a valid UUID, so use it only with compile-time constants
// and in tests.
func MustParseVirtualKubernetesClusterID(s string) VirtualKubernetesClusterID {
	return VirtualKubernetesClusterID(uuid.MustParse(s))
}

// MustParseClusterManagerID parses s as a UUID into a ClusterManagerID.
// It panics if s is not a valid UUID, so use it only with compile-time constants
// and in tests.
func MustParseClusterManagerID(s string) ClusterManagerID {
	return ClusterManagerID(uuid.MustParse(s))
}
