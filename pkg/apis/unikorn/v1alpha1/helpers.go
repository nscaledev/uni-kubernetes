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

package v1alpha1

import (
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/core/pkg/constants"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var (
	// ErrMissingLabel is raised when an expected label is not present on
	// a resource.
	ErrMissingLabel = errors.New("expected label is missing")

	// ErrApplicationLookup is raised when the named application is not
	// present in an application bundle bundle.
	ErrApplicationLookup = errors.New("failed to lookup an application")
)

// All implements generic iteration over list items.
func (l *ClusterManagerList) All() iter.Seq[*ClusterManager] {
	return func(yield func(t *ClusterManager) bool) {
		for i := range l.Items {
			if !yield(&l.Items[i]) {
				return
			}
		}
	}
}

// All implements generic iteration over list items.
func (l *KubernetesClusterList) All() iter.Seq[*KubernetesCluster] {
	return func(yield func(t *KubernetesCluster) bool) {
		for i := range l.Items {
			if !yield(&l.Items[i]) {
				return
			}
		}
	}
}

// All implements generic iteration over list items.
func (l *VirtualKubernetesClusterList) All() iter.Seq[*VirtualKubernetesCluster] {
	return func(yield func(t *VirtualKubernetesCluster) bool) {
		for i := range l.Items {
			if !yield(&l.Items[i]) {
				return
			}
		}
	}
}

// Paused implements the ReconcilePauser interface.
func (c *ClusterManager) Paused() bool {
	return c.Spec.Pause
}

// Paused implements the ReconcilePauser interface.
func (c *KubernetesCluster) Paused() bool {
	return c.Spec.Pause
}

// Paused implements the ReconcilePauser interface.
func (c *VirtualKubernetesCluster) Paused() bool {
	return c.Spec.Pause
}

// StatusConditionRead scans the status conditions for an existing condition whose type
// matches.
func (c *ClusterManager) StatusConditionRead(t unikornv1core.ConditionType) (*unikornv1core.Condition, error) {
	return unikornv1core.GetCondition(c.Status.Conditions, t)
}

// StatusConditionWrite either adds or updates a condition in the cluster manager status.
// If the condition, status and message match an existing condition the update is
// ignored.
func (c *ClusterManager) StatusConditionWrite(t unikornv1core.ConditionType, status corev1.ConditionStatus, reason unikornv1core.ConditionReason, message string) {
	unikornv1core.UpdateCondition(&c.Status.Conditions, t, status, reason, message)
}

// ResourceLabels generates a set of labels to uniquely identify the resource
// if it were to be placed in a single global namespace.
func (c *ClusterManager) ResourceLabels() (labels.Set, error) {
	organization, ok := c.Labels[constants.OrganizationLabel]
	if !ok {
		return nil, ErrMissingLabel
	}

	project, ok := c.Labels[constants.ProjectLabel]
	if !ok {
		return nil, ErrMissingLabel
	}

	labels := labels.Set{
		constants.KindLabel:         constants.KindLabelValueClusterManager,
		constants.OrganizationLabel: organization,
		constants.ProjectLabel:      project,
	}

	return labels, nil
}

func (c *ClusterManager) Entropy() []byte {
	return []byte(c.UID)
}

func (c *ClusterManager) UpgradeSpec() *ApplicationBundleAutoUpgradeSpec {
	return c.Spec.ApplicationBundleAutoUpgrade
}

// StatusConditionRead scans the status conditions for an existing condition whose type
// matches.
func (c *KubernetesCluster) StatusConditionRead(t unikornv1core.ConditionType) (*unikornv1core.Condition, error) {
	return unikornv1core.GetCondition(c.Status.Conditions, t)
}

// StatusConditionWrite either adds or updates a condition in the cluster status.
// If the condition, status and message match an existing condition the update is
// ignored.
func (c *KubernetesCluster) StatusConditionWrite(t unikornv1core.ConditionType, status corev1.ConditionStatus, reason unikornv1core.ConditionReason, message string) {
	unikornv1core.UpdateCondition(&c.Status.Conditions, t, status, reason, message)
}

// ResourceLabels generates a set of labels to uniquely identify the resource
// if it were to be placed in a single global namespace.
func (c *KubernetesCluster) ResourceLabels() (labels.Set, error) {
	organization, ok := c.Labels[constants.OrganizationLabel]
	if !ok {
		return nil, ErrMissingLabel
	}

	project, ok := c.Labels[constants.ProjectLabel]
	if !ok {
		return nil, ErrMissingLabel
	}

	labels := labels.Set{
		constants.KindLabel:              constants.KindLabelValueKubernetesCluster,
		constants.OrganizationLabel:      organization,
		constants.ProjectLabel:           project,
		constants.KubernetesClusterLabel: c.Name,
	}

	return labels, nil
}

func (c *KubernetesCluster) Entropy() []byte {
	return []byte(c.UID)
}

func (c *KubernetesCluster) UpgradeSpec() *ApplicationBundleAutoUpgradeSpec {
	return c.Spec.ApplicationBundleAutoUpgrade
}

// AutoscalingEnabled indicates whether cluster autoscaling is enabled for the cluster.
func (c *KubernetesCluster) AutoscalingEnabled() bool {
	return c.Spec.Features != nil && c.Spec.Features.Autoscaling
}

// GPUOperatorEnabled indicates whether to install the GPU operator.
func (c *KubernetesCluster) GPUOperatorEnabled() bool {
	return c.Spec.Features != nil && c.Spec.Features.GPUOperator
}

func (c *KubernetesCluster) GetWorkloadPool(name string) *KubernetesWorkloadPoolSpec {
	for i, pool := range c.Spec.WorkloadPools.Pools {
		if pool.Name == name {
			return &c.Spec.WorkloadPools.Pools[i]
		}
	}

	return nil
}

// StatusConditionRead scans the status conditions for an existing condition whose type
// matches.
func (c *VirtualKubernetesCluster) StatusConditionRead(t unikornv1core.ConditionType) (*unikornv1core.Condition, error) {
	return unikornv1core.GetCondition(c.Status.Conditions, t)
}

// StatusConditionWrite either adds or updates a condition in the cluster status.
// If the condition, status and message match an existing condition the update is
// ignored.
func (c *VirtualKubernetesCluster) StatusConditionWrite(t unikornv1core.ConditionType, status corev1.ConditionStatus, reason unikornv1core.ConditionReason, message string) {
	unikornv1core.UpdateCondition(&c.Status.Conditions, t, status, reason, message)
}

// ResourceLabels generates a set of labels to uniquely identify the resource
// if it were to be placed in a single global namespace.
func (c *VirtualKubernetesCluster) ResourceLabels() (labels.Set, error) {
	organization, ok := c.Labels[constants.OrganizationLabel]
	if !ok {
		return nil, ErrMissingLabel
	}

	project, ok := c.Labels[constants.ProjectLabel]
	if !ok {
		return nil, ErrMissingLabel
	}

	labels := labels.Set{
		constants.KindLabel:              constants.KindLabelValueVirtualKubernetesCluster,
		constants.OrganizationLabel:      organization,
		constants.ProjectLabel:           project,
		constants.KubernetesClusterLabel: c.Name,
	}

	return labels, nil
}

func CompareClusterManager(a, b ClusterManager) int {
	return strings.Compare(a.Name, b.Name)
}

func CompareKubernetesCluster(a, b KubernetesCluster) int {
	return strings.Compare(a.Name, b.Name)
}

func CompareVirtualKubernetesCluster(a, b VirtualKubernetesCluster) int {
	return strings.Compare(a.Name, b.Name)
}

func CompareClusterManagerApplicationBundle(a, b ClusterManagerApplicationBundle) int {
	return a.Spec.Version.Compare(&b.Spec.Version)
}

func CompareKubernetesClusterApplicationBundle(a, b KubernetesClusterApplicationBundle) int {
	return a.Spec.Version.Compare(&b.Spec.Version)
}

func CompareVirtualKubernetesClusterApplicationBundle(a, b VirtualKubernetesClusterApplicationBundle) int {
	return a.Spec.Version.Compare(&b.Spec.Version)
}

// Get retrieves the named bundle.
func (l *ClusterManagerApplicationBundleList) Get(name string) *ClusterManagerApplicationBundle {
	for i := range l.Items {
		if l.Items[i].Name == name {
			return &l.Items[i]
		}
	}

	return nil
}

func (l *KubernetesClusterApplicationBundleList) Get(name string) *KubernetesClusterApplicationBundle {
	for i := range l.Items {
		if l.Items[i].Name == name {
			return &l.Items[i]
		}
	}

	return nil
}

// Upgradable returns a new list of bundles that are "stable" e.g. not end of life and
// not a preview.
func (l *ClusterManagerApplicationBundleList) Upgradable() *ClusterManagerApplicationBundleList {
	result := &ClusterManagerApplicationBundleList{}

	for _, bundle := range l.Items {
		if bundle.Spec.Preview {
			continue
		}

		if bundle.Spec.EndOfLife != nil {
			continue
		}

		result.Items = append(result.Items, bundle)
	}

	return result
}

func (l *KubernetesClusterApplicationBundleList) Upgradable() *KubernetesClusterApplicationBundleList {
	result := &KubernetesClusterApplicationBundleList{}

	for _, bundle := range l.Items {
		if bundle.Spec.Preview {
			continue
		}

		if bundle.Spec.EndOfLife != nil {
			continue
		}

		result.Items = append(result.Items, bundle)
	}

	return result
}

func (s *ApplicationBundleSpec) GetApplication(name string) (*unikornv1core.ApplicationReference, error) {
	for i := range s.Applications {
		if s.Applications[i].Name == name {
			return &s.Applications[i].Reference, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrApplicationLookup, name)
}

// Weekdays returns the days of the week that are set in the spec.
func (s *ApplicationBundleAutoUpgradeWeekDaySpec) Weekdays() []time.Weekday {
	var result []time.Weekday

	if s.Sunday != nil {
		result = append(result, time.Sunday)
	}

	if s.Monday != nil {
		result = append(result, time.Monday)
	}

	if s.Tuesday != nil {
		result = append(result, time.Tuesday)
	}

	if s.Wednesday != nil {
		result = append(result, time.Wednesday)
	}

	if s.Thursday != nil {
		result = append(result, time.Thursday)
	}

	if s.Friday != nil {
		result = append(result, time.Friday)
	}

	if s.Saturday != nil {
		result = append(result, time.Saturday)
	}

	return result
}
