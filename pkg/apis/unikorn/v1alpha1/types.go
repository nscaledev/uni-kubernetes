/*
Copyright 2022-2024 EscherCloud.
Copyright 2024 the Unikorn Authors.

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

package v1alpha1

import (
	"encoding/json"
	"errors"
	"net"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/structured-merge-diff/v4/value"
)

var (
	ErrJSONUnmarshal = errors.New("failed to unmarshal JSON")
)

// +kubebuilder:validation:Pattern="^v(?:[0-9]+\\.){2}(?:[0-9]+)$"
type SemanticVersion string

// +kubebuilder:validation:Type=string
// +kubebuilder:validation:Pattern="^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])\\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])$"
type IPv4Address struct {
	net.IP
}

// Ensure the type implements json.Unmarshaler.
var _ = json.Unmarshaler(&IPv4Address{})

func (a *IPv4Address) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}

	ip := net.ParseIP(str)
	if ip == nil {
		return ErrJSONUnmarshal
	}

	a.IP = ip

	return nil
}

// Ensure the type implements value.UnstructuredConverter.
var _ = value.UnstructuredConverter(&IPv4Address{})

func (a IPv4Address) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.IP.String())
}

func (a IPv4Address) ToUnstructured() interface{} {
	return a.IP.String()
}

// There is no interface defined for these. See
// https://github.com/kubernetes/kube-openapi/tree/master/pkg/generators
// for reference.
func (IPv4Address) OpenAPISchemaType() []string {
	return []string{"string"}
}

func (IPv4Address) OpenAPISchemaFormat() string {
	return ""
}

// See https://regex101.com/r/QUfWrF/1
// +kubebuilder:validation:Type=string
// +kubebuilder:validation:Pattern="^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])\\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])\\/(?:3[0-2]|[1-2]?[0-9])$"
type IPv4Prefix struct {
	net.IPNet
}

// DeepCopyInto implements the interface deepcopy-gen is totally unable to
// do by itself.
func (p *IPv4Prefix) DeepCopyInto(out *IPv4Prefix) {
	if p.IPNet.IP != nil {
		in, out := &p.IPNet.IP, &out.IPNet.IP
		*out = make(net.IP, len(*in))
		copy(*out, *in)
	}

	if p.IPNet.Mask != nil {
		in, out := &p.IPNet.Mask, &out.IPNet.Mask
		*out = make(net.IPMask, len(*in))
		copy(*out, *in)
	}
}

// Ensure the type implements json.Unmarshaler.
var _ = json.Unmarshaler(&IPv4Prefix{})

func (p *IPv4Prefix) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}

	_, network, err := net.ParseCIDR(str)
	if err != nil {
		return ErrJSONUnmarshal
	}

	if network == nil {
		return ErrJSONUnmarshal
	}

	p.IPNet = *network

	return nil
}

// Ensure the type implements value.UnstructuredConverter.
var _ = value.UnstructuredConverter(&IPv4Prefix{})

func (p IPv4Prefix) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.IPNet.String())
}

func (p IPv4Prefix) ToUnstructured() interface{} {
	return p.IP.String()
}

// There is no interface defined for these. See
// https://github.com/kubernetes/kube-openapi/tree/master/pkg/generators
// for reference.
func (IPv4Prefix) OpenAPISchemaType() []string {
	return []string{"string"}
}

func (IPv4Prefix) OpenAPISchemaFormat() string {
	return ""
}

// ClusterManagerList is a typed list of cluster managers.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterManager `json:"items"`
}

// ClusterManager is an abstraction around resource provisioning, for example
// it may contain a provider like Cluster API that can provision KubernetesCluster
// resources.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Namespaced,categories=unikorn
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="display name",type="string",JSONPath=".metadata.labels['unikorn-cloud\\.org/name']"
// +kubebuilder:printcolumn:name="bundle",type="string",JSONPath=".spec.applicationBundle"
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.conditions[?(@.type==\"Available\")].reason"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type ClusterManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterManagerSpec   `json:"spec"`
	Status            ClusterManagerStatus `json:"status,omitempty"`
}

// ClusterManagerSpec defines any cluster manager specific options.
type ClusterManagerSpec struct {
	// Pause, if true, will inhibit reconciliation.
	Pause bool `json:"pause,omitempty"`
	// ApplicationBundle defines the applications used to create the cluster manager.
	// Change this to a new bundle to start an upgrade.
	ApplicationBundle *string `json:"applicationBundle"`
	// ApplicationBundleAutoUpgrade enables automatic upgrade of application bundles.
	// When no properties are set in the specification, the platform will automatically
	// choose an upgrade time for your resource.  This will be before a working day
	// (Mon-Fri) and before working hours (00:00-07:00 UTC).  When any property is set
	// the platform will follow the rules for the upgrade method.
	ApplicationBundleAutoUpgrade *ApplicationBundleAutoUpgradeSpec `json:"applicationBundleAutoUpgrade,omitempty"`
}

// ClusterManagerStatus defines the status of the project.
type ClusterManagerStatus struct {
	// Current service state of a cluster manager.
	Conditions []unikornv1core.Condition `json:"conditions,omitempty"`
}

// MachineGeneric contains common things across all pool types, including
// Kubernetes cluster manager nodes and workload pools.
type MachineGeneric struct {
	// Image is the OpenStack Glance image to deploy with.
	ImageID *string `json:"imageId"`
	// Flavor is the OpenStack Nova flavor to deploy with.
	FlavorID *string `json:"flavorId"`
	// FlavorName is the name of the flavor.
	// CAPO is broken and doesn't accept an ID, so we need to use this.
	FlavorName *string `json:"flavorName"`
	// DiskSize is the persistent root disk size to deploy with.  This
	// overrides the default ephemeral disk size defined in the flavor.
	DiskSize *resource.Quantity `json:"diskSize,omitempty"`
	// Replicas is the initial pool size to deploy.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=3
	Replicas *int `json:"replicas,omitempty"`
}

// File is a file that can be deployed to a cluster node on creation.
type File struct {
	// Path is the absolute path to create the file in.
	Path *string `json:"path"`
	// Content is the file contents.
	Content []byte `json:"content"`
}

// MachineGenericAutoscaling defines generic autoscaling configuration.
// +kubebuilder:validation:XValidation:message="maximumReplicas must be greater than minimumReplicas",rule=(self.maximumReplicas > self.minimumReplicas)
type MachineGenericAutoscaling struct {
	// MinimumReplicas defines the minimum number of replicas that
	// this pool can be scaled down to.
	// +kubebuilder:validation:Minimum=0
	MinimumReplicas *int `json:"minimumReplicas"`
	// MaximumReplicas defines the maximum numer of replicas that
	// this pool can be scaled up to.
	// +kubebuilder:validation:Minimum=1
	MaximumReplicas *int `json:"maximumReplicas"`
	// Scheduler is required when scale-from-zero support is requested
	// i.e. MimumumReplicas is 0.  This provides scheduling hints to
	// the autoscaler as it cannot derive CPU/memory constraints from
	// the machine flavor.
	Scheduler *MachineGenericAutoscalingScheduler `json:"scheduler,omitempty"`
}

// MachineGenericAutoscalingScheduler defines generic autoscaling scheduling
// constraints.
type MachineGenericAutoscalingScheduler struct {
	// CPU defines the number of CPUs for the pool flavor.
	// +kubebuilder:validation:Minimum=1
	CPU *int `json:"cpu"`
	// Memory defines the amount of memory for the pool flavor.
	// Internally this will be rounded down to the nearest Gi.
	Memory *resource.Quantity `json:"memory"`
	// GPU needs to be set when the pool contains GPU resources so
	// the autoscaler can make informed choices when scaling up.
	GPU *MachineGenericAutoscalingSchedulerGPU `json:"gpu,omitempty"`
}

// MachineGenericAutoscalingSchedulerGPU defines generic autoscaling
// scheduling constraints for GPUs.
type MachineGenericAutoscalingSchedulerGPU struct {
	// Type is the type of GPU.
	// +kubebuilder:validation:Enum=nvidia.com/gpu
	Type *string `json:"type"`
	// Count is the number of GPUs for the pool flavor.
	// +kubebuilder:validation:Minimum=1
	Count *int `json:"count"`
}

// KubernetesWorkloadPoolSpec defines the requested machine pool
// state.
type KubernetesWorkloadPoolSpec struct {
	MachineGeneric `json:",inline"`
	// Name is the name of the pool.
	Name string `json:"name"`
	// Labels is the set of node labels to apply to the pool on
	// initialisation/join.
	Labels map[string]string `json:"labels,omitempty"`
	// Files are a set of files that can be installed onto the node
	// on initialisation/join.
	Files []File `json:"files,omitempty"`
	// Autoscaling contains optional sclaing limits and scheduling
	// hints for autoscaling.
	Autoscaling *MachineGenericAutoscaling `json:"autoscaling,omitempty"`
}

// KubernetesClusterList is a typed list of kubernetes clusters.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type KubernetesClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubernetesCluster `json:"items"`
}

// KubernetesCluster is an object representing a Kubernetes cluster.
// For now, this is a monolith for simplicity.  In future it may reference
// a provider specific implementation e.g. if CAPI goes out of favour for
// some other new starlet.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Namespaced,categories=unikorn
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="display name",type="string",JSONPath=".metadata.labels['unikorn-cloud\\.org/name']"
// +kubebuilder:printcolumn:name="bundle",type="string",JSONPath=".spec.applicationBundle"
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.version"
// +kubebuilder:printcolumn:name="status",type="string",JSONPath=".status.conditions[?(@.type==\"Available\")].reason"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type KubernetesCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KubernetesClusterSpec   `json:"spec"`
	Status            KubernetesClusterStatus `json:"status,omitempty"`
}

// KubernetesClusterSpec defines the requested state of the Kubernetes cluster.
type KubernetesClusterSpec struct {
	// Pause, if true, will inhibit reconciliation.
	Pause bool `json:"pause,omitempty"`
	// Region to provision the cluster in.
	RegionID string `json:"regionId"`
	// ClusterManager that provides lifecycle management for the cluster.
	ClusterManagerID string `json:"clusterManagerId"`
	// Version is the Kubernetes version to install.  For performance
	// reasons this should match what is already pre-installed on the
	// provided image.
	Version *SemanticVersion `json:"version"`
	// Network defines the Kubernetes networking.
	Network *KubernetesClusterNetworkSpec `json:"network"`
	// API defines Kubernetes API specific options.
	API *KubernetesClusterAPISpec `json:"api,omitempty"`
	// ControlPlane defines the cluster manager topology.
	ControlPlane *KubernetesClusterControlPlaneSpec `json:"controlPlane"`
	// WorkloadPools defines the workload cluster topology.
	WorkloadPools *KubernetesClusterWorkloadPoolsSpec `json:"workloadPools"`
	// Features defines add-on features that can be enabled for the cluster.
	Features *KubernetesClusterFeaturesSpec `json:"features,omitempty"`
	// ApplicationBundle defines the applications used to create the cluster.
	// Change this to a new bundle to start an upgrade.
	ApplicationBundle *string `json:"applicationBundle"`
	// ApplicationBundleAutoUpgrade enables automatic upgrade of application bundles.
	// When no properties are set in the specification, the platform will automatically
	// choose an upgrade time for your resource.  This will be before a working day
	// (Mon-Fri) and before working hours (00:00-07:00 UTC).  When any property is set
	// the platform will follow the rules for the upgrade method.
	ApplicationBundleAutoUpgrade *ApplicationBundleAutoUpgradeSpec `json:"applicationBundleAutoUpgrade,omitempty"`
}

type KubernetesClusterAPISpec struct {
	// SubjectAlternativeNames is a list of X.509 SANs to add to the API
	// certificate.
	SubjectAlternativeNames []string `json:"subjectAlternativeNames,omitempty"`
	// AllowedPrefixes is a list of all IPv4 prefixes that are allowed to access
	// the API.
	AllowedPrefixes []IPv4Prefix `json:"allowedPrefixes,omitempty"`
}

type KubernetesClusterNetworkSpec struct {
	// NodeNetwork is the IPv4 prefix for the node network.
	NodeNetwork *IPv4Prefix `json:"nodeNetwork"`
	// PodNetwork is the IPv4 prefix for the pod network.
	PodNetwork *IPv4Prefix `json:"podNetwork"`
	// ServiceNetwork is the IPv4 prefix for the service network.
	ServiceNetwork *IPv4Prefix `json:"serviceNetwork"`
	// DNSNameservers sets the DNS nameservers for pods.
	// At present due to some technical challenges, this must contain
	// only one DNS server.
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	DNSNameservers []IPv4Address `json:"dnsNameservers"`
}

type KubernetesClusterFeaturesSpec struct {
	// Autoscaling, if true, provisions a cluster autoscaler
	// and allows workload pools to specify autoscaling configuration.
	Autoscaling *bool `json:"autoscaling,omitempty"`
	// NvidiaOperator, if false do not install the Nvidia Operator, otherwise
	// install if GPU flavors are detected
	NvidiaOperator *bool `json:"nvidiaOperator,omitempty"`
}

type KubernetesClusterControlPlaneSpec struct {
	MachineGeneric `json:",inline"`
}

type KubernetesClusterWorkloadPoolsPoolSpec struct {
	KubernetesWorkloadPoolSpec `json:",inline"`
}

type KubernetesClusterWorkloadPoolsSpec struct {
	// Pools contains an inline set of pools.  This field will be ignored
	// when Selector is set.  Inline pools are expected to be used for UI
	// generated clusters.
	Pools []KubernetesClusterWorkloadPoolsPoolSpec `json:"pools,omitempty"`
}

// KubernetesClusterStatus defines the observed state of the Kubernetes cluster.
type KubernetesClusterStatus struct {
	// Namespace defines the namespace a cluster resides in.
	Namespace string `json:"namespace,omitempty"`

	// Current service state of a Kubernetes cluster.
	Conditions []unikornv1core.Condition `json:"conditions,omitempty"`
}

// ClusterManagerApplicationBundleList defines a list of application bundles.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterManagerApplicationBundleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterManagerApplicationBundle `json:"items"`
}

// ClusterManagerApplicationBundle defines a bundle of applications related with a particular custom
// resource e.g. a ClusterManager has vcluster, cert-manager and cluster-api applications
// associated with it.  This forms the backbone of upgrades by allowing bundles to be
// switched out in cluster managers etc.
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster,categories=unikorn
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.version"
// +kubebuilder:printcolumn:name="preview",type="string",JSONPath=".spec.preview"
// +kubebuilder:printcolumn:name="end of life",type="string",JSONPath=".spec.endOfLife"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type ClusterManagerApplicationBundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ApplicationBundleSpec   `json:"spec"`
	Status            ApplicationBundleStatus `json:"status,omitempty"`
}

// KubernetesClusterApplicationBundleList defines a list of application bundles.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type KubernetesClusterApplicationBundleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubernetesClusterApplicationBundle `json:"items"`
}

// KubernetesClusterApplicationBundle defines a bundle of applications related with a particular custom
// resource e.g. a ClusterManager has vcluster, cert-manager and cluster-api applications
// associated with it.  This forms the backbone of upgrades by allowing bundles to be
// switched out in cluster managers etc.
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster,categories=unikorn
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".spec.version"
// +kubebuilder:printcolumn:name="preview",type="string",JSONPath=".spec.preview"
// +kubebuilder:printcolumn:name="end of life",type="string",JSONPath=".spec.endOfLife"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type KubernetesClusterApplicationBundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ApplicationBundleSpec   `json:"spec"`
	Status            ApplicationBundleStatus `json:"status,omitempty"`
}

// ApplicationBundleSpec defines the requested resource state.
type ApplicationBundleSpec struct {
	// Version is a semantic version of the bundle, must be unique.
	Version *string `json:"version"`
	// Preview indicates that this bundle is a preview and should not be
	// used by default.
	Preview *bool `json:"preview,omitempty"`
	// EndOfLife marks when this bundle should not be advertised any more
	// by Unikorn server.  It also provides a hint that users should upgrade
	// ahead of the deadline, or that a forced upgrade should be triggered.
	EndOfLife *metav1.Time `json:"endOfLife,omitempty"`
	// Applications is a list of application references for the bundle.
	Applications []ApplicationNamedReference `json:"applications,omitempty"`
}

type ApplicationNamedReference struct {
	// Name is the name of the application.  This must match what is encoded into
	// Unikorn's application management engine.
	Name *string `json:"name"`
	// Reference is a reference to the application definition.
	Reference *unikornv1core.ApplicationReference `json:"reference"`
}

type ApplicationBundleStatus struct{}

type ApplicationBundleAutoUpgradeSpec struct {
	// WeekDay allows specification of upgrade time windows on individual
	// days of the week.  The platform will select a random  upgrade
	// slot within the specified time windows in order to load balance and
	// mitigate against defects.
	WeekDay *ApplicationBundleAutoUpgradeWeekDaySpec `json:"weekday,omitempty"`
}

type ApplicationBundleAutoUpgradeWeekDaySpec struct {
	// Sunday, when specified, provides an upgrade window on that day.
	Sunday *ApplicationBundleAutoUpgradeWindowSpec `json:"sunday,omitempty"`
	// Monday, when specified, provides an upgrade window on that day.
	Monday *ApplicationBundleAutoUpgradeWindowSpec `json:"monday,omitempty"`
	// Tuesday, when specified, provides an upgrade window on that day.
	Tuesday *ApplicationBundleAutoUpgradeWindowSpec `json:"tuesday,omitempty"`
	// Wednesday, when specified, provides an upgrade window on that day.
	Wednesday *ApplicationBundleAutoUpgradeWindowSpec `json:"wednesday,omitempty"`
	// Thursday, when specified, provides an upgrade window on that day.
	Thursday *ApplicationBundleAutoUpgradeWindowSpec `json:"thursday,omitempty"`
	// Friday, when specified, provides an upgrade window on that day.
	Friday *ApplicationBundleAutoUpgradeWindowSpec `json:"friday,omitempty"`
	// Saturday, when specified, provides an upgrade window on that day.
	Saturday *ApplicationBundleAutoUpgradeWindowSpec `json:"saturday,omitempty"`
}

type ApplicationBundleAutoUpgradeWindowSpec struct {
	// Start is the upgrade window start hour in UTC.  Upgrades will be
	// deterministically scheduled between start and end to balance load
	// across the platform.  Windows can span days, so start=22 and end=07
	// will start at 22:00 on the selected day, and end 07:00 the following
	// one.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=23
	Start int `json:"start"`
	// End is the upgrade window end hour in UTC.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=23
	End int `json:"end"`
}
