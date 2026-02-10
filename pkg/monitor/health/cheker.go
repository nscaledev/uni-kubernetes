/*
Copyright 2025 the Unikorn Authors.
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

package health

import (
	"context"
	"errors"
	"fmt"
	"iter"

	unikornv1 "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/core/pkg/cd"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrTypeConversion = errors.New("type conversion error")
)

// Lister is a generic inteface for operating on and iterating over resource
// lists (as no such interface is required by apimachinery.
type Lister[T unikornv1.ManagableResourceInterface] interface {
	client.ObjectList
	All() iter.Seq[T]
}

// Checker lists all resources of the specified type and does a health check on it.
// The type itself is constrained to a manageable resource so we can get the label selector
// to pass to the CD layer to get all applications for the resource, then once we have
// checked the status of those applications we can set the condition generically, again
// as provided by the manageable resource interface.
type Checker[T unikornv1.ManagableResourceInterface, L Lister[T]] struct {
	// client allows access to Kubernetes resources.
	client client.Client
	// driver is the CD driver.
	driver cd.Driver
	// l is storage for the manageable resource list.
	l L
}

// New creates a new checker.  All types can be inferred, the template parameters
// are purely for type constraints.
func New[T unikornv1.ManagableResourceInterface, L Lister[T]](client client.Client, driver cd.Driver, l L) *Checker[T, L] {
	return &Checker[T, L]{
		client: client,
		driver: driver,
		l:      l,
	}
}

// resourceIdentifierFromResource takes our manageable resource type and returns
// a CD resource ID to identify its applications.
func resourceIdentifierFromResource(r unikornv1.ManagableResourceInterface) (*cd.ResourceIdentifier, error) {
	labels, err := r.ResourceLabels()
	if err != nil {
		return nil, err
	}

	id := &cd.ResourceIdentifier{
		Labels: make([]cd.ResourceIdentifierLabel, 0, len(labels)),
	}

	for k, v := range labels {
		id.Labels = append(id.Labels, cd.ResourceIdentifierLabel{
			Name:  k,
			Value: v,
		})
	}

	return id, nil
}

// convertHealthStatus translates from the CD interface to the Kubernetes API.
func convertHealthStatus(status cd.HealthStatus) (corev1.ConditionStatus, unikornv1.ConditionReason, string) {
	switch status {
	case cd.HealthStatusUnknown:
		return corev1.ConditionUnknown, unikornv1.ConditionReasonUnknown, "unable to poll application status"
	case cd.HealthStatusHealthy:
		return corev1.ConditionTrue, unikornv1.ConditionReasonHealthy, "resource applications healthy"
	case cd.HealthStatusDegraded:
		return corev1.ConditionFalse, unikornv1.ConditionReasonDegraded, "one or more resource applications are degraded"
	}

	// NOTE: the linter will warn about non-exhaustive switches.
	return corev1.ConditionUnknown, unikornv1.ConditionReasonUnknown, "unreachable code reached"
}

// check does the actual check for a resource and updates its status.
func (c *Checker[T, L]) check(ctx context.Context, r unikornv1.ManagableResourceInterface) error {
	// Grab the overall health status.
	id, err := resourceIdentifierFromResource(r)
	if err != nil {
		return err
	}

	// TODO: we only support argo now, but will need an abstraction down the line.
	// There is precedent in the main controllers.
	healthStatus, err := c.driver.GetHealthStatus(ctx, id)
	if err != nil {
		return err
	}

	updated, ok := r.DeepCopyObject().(unikornv1.ManagableResourceInterface)
	if !ok {
		return fmt.Errorf("%w: unable to deep copy manageable resource", ErrTypeConversion)
	}

	// And finally set the status condition.
	status, reason, message := convertHealthStatus(healthStatus)

	updated.StatusConditionWrite(unikornv1.ConditionHealthy, status, reason, message)

	if err := c.client.Status().Patch(ctx, updated, client.MergeFromWithOptions(r, &client.MergeFromWithOptimisticLock{})); err != nil {
		return err
	}

	return nil
}

// Check does the actual check as described for the health checker type.
func (c *Checker[T, L]) Check(ctx context.Context) error {
	// NOTE: This looks expensive, but it's all cached by controller-runtime.
	if err := c.client.List(ctx, c.l, &client.ListOptions{}); err != nil {
		return err
	}

	for o := range c.l.All() {
		if err := c.check(ctx, o); err != nil {
			return err
		}
	}

	return nil
}
