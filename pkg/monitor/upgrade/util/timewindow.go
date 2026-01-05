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

package util

import (
	"context"
	"crypto/sha256"
	"time"

	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// UpgradeableResource is a resource type that allows upgrades.
type UpgradeableResource interface {
	// Entropy returns a unique and random source of entropy from
	// the resources.
	Entropy() []byte

	// UpgradeSpec is the upgrade specification.
	UpgradeSpec() *unikornv1.ApplicationBundleAutoUpgradeSpec
}

// forcedUpgradeResource wraps an existing resource that didn't opt in
// to upgrades, but implicitly adds auto upgrade.
type forcedUpgradeResource struct {
	r UpgradeableResource
}

func NewForcedUpgradeResource(r UpgradeableResource) UpgradeableResource {
	return &forcedUpgradeResource{r: r}
}

func (r *forcedUpgradeResource) Entropy() []byte {
	return r.r.Entropy()
}

func (r *forcedUpgradeResource) UpgradeSpec() *unikornv1.ApplicationBundleAutoUpgradeSpec {
	return &unikornv1.ApplicationBundleAutoUpgradeSpec{}
}

type TimeWindow struct {
	Start time.Time
	End   time.Time
}

// In tells us if we are in the time window.
func (t *TimeWindow) In() bool {
	if t == nil {
		return false
	}

	now := time.Now()

	return now.After(t.Start) && now.Before(t.End)
}

// GenerateTimeWindow returns a one hour time window in which to trigger a
// resource upgrade.
func autoTimeWindow(ctx context.Context, r UpgradeableResource) *TimeWindow {
	log := log.FromContext(ctx)

	log.Info("auto generating time window")

	// Run upgrades Monday-Friday, from 00:00 to 07:00, so we'll be in the
	// office shortly after anything goes wrong.  7AM gives us enough leeway
	// to get stuff done before 9AM CEST.
	config := &unikornv1.ApplicationBundleAutoUpgradeWeekDaySpec{
		Monday: &unikornv1.ApplicationBundleAutoUpgradeWindowSpec{
			Start: 0,
			End:   7,
		},
		Tuesday: &unikornv1.ApplicationBundleAutoUpgradeWindowSpec{
			Start: 0,
			End:   7,
		},
		Wednesday: &unikornv1.ApplicationBundleAutoUpgradeWindowSpec{
			Start: 0,
			End:   7,
		},
		Thursday: &unikornv1.ApplicationBundleAutoUpgradeWindowSpec{
			Start: 0,
			End:   7,
		},
		Friday: &unikornv1.ApplicationBundleAutoUpgradeWindowSpec{
			Start: 0,
			End:   7,
		},
	}

	return weekDayTimeWindow(ctx, r, config)
}

func weekDayTimeWindow(ctx context.Context, r UpgradeableResource, weekday *unikornv1.ApplicationBundleAutoUpgradeWeekDaySpec) *TimeWindow {
	log := log.FromContext(ctx)

	log.Info("using day of the week time window")

	// We pick one day out of the provided ones, this reduces upgrade traffic
	// and load balances it across all the provided days.
	sum := sha256.Sum256(r.Entropy())

	available := weekday.Weekdays()
	selected := available[int(sum[0])%len(available)]

	// window is the selected day's window.
	var window *unikornv1.ApplicationBundleAutoUpgradeWindowSpec

	switch selected {
	case time.Sunday:
		window = weekday.Sunday
	case time.Monday:
		window = weekday.Monday
	case time.Tuesday:
		window = weekday.Tuesday
	case time.Wednesday:
		window = weekday.Wednesday
	case time.Thursday:
		window = weekday.Thursday
	case time.Friday:
		window = weekday.Friday
	case time.Saturday:
		window = weekday.Saturday
	}

	// Select a random hour from the time window and use that to perform the
	// upgrade, again for load balancing and support reasons as stated.
	windowLength := window.End - window.Start
	if windowLength <= 0 {
		windowLength += 24
	}

	windowHour := int(sum[1]) % windowLength

	log.Info("selected window", "available", available, "selected", selected, "window_start", window.Start, "window_end", window.End, "selected_start", window.Start+windowHour)

	now := time.Now()

	start := time.Date(now.Year(), now.Month(), now.Day()-int(now.Weekday())+int(selected), window.Start+windowHour, 0, 0, 0, time.UTC)

	return &TimeWindow{Start: start, End: start.Add(time.Hour)}
}

func TimeWindowFromResource(ctx context.Context, r UpgradeableResource) *TimeWindow {
	spec := r.UpgradeSpec()

	switch {
	case spec.WeekDay != nil:
		return weekDayTimeWindow(ctx, r, spec.WeekDay)
	default:
		return autoTimeWindow(ctx, r)
	}
}
