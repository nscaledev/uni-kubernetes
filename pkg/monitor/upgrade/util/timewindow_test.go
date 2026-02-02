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

package util_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/monitor/upgrade/util"
)

const (
	samples = 1000000
)

// logSink provides logr compatible logging for testing.
type logSink struct {
	t *testing.T
}

func (s *logSink) Init(_ logr.RuntimeInfo) {
}

func (s *logSink) Enabled(_ int) bool {
	return true
}

func (s *logSink) Info(_ int, msg string, kv ...any) {
	args := []any{msg}
	args = append(args, kv...)

	s.t.Log(args...)
}

func (s *logSink) Error(err error, msg string, kv ...any) {
	args := []any{err, msg}
	args = append(args, kv...)

	s.t.Log(args...)
}

func (s *logSink) WithValues(_ ...any) logr.LogSink {
	return s
}

func (s *logSink) WithName(_ string) logr.LogSink {
	return s
}

// testContext returns a context with a logger attached.
func testContext(t *testing.T) context.Context {
	t.Helper()

	return logr.NewContext(t.Context(), logr.New(&logSink{t: t}))
}

// autoGeneratingUpgrader provides a "resource" that has auto upgrade
// scheduling enabled.
type autoGeneratingUpgrader struct{}

func (u autoGeneratingUpgrader) Entropy() []byte {
	return []byte(uuid.New().String())
}

func (u autoGeneratingUpgrader) UpgradeSpec() *unikornv1.ApplicationBundleAutoUpgradeSpec {
	return &unikornv1.ApplicationBundleAutoUpgradeSpec{}
}

// weekDayUpgrader provides as "resource" that has per-weekday upgrade
// scheduling enabled.
type weekDayUpgrader struct {
	day   time.Weekday
	start int
	end   int
}

func newWeekDayUpgrader(day time.Weekday, start, end int) *weekDayUpgrader {
	return &weekDayUpgrader{
		day:   day,
		start: start,
		end:   end,
	}
}

func (u weekDayUpgrader) Entropy() []byte {
	return []byte(uuid.New().String())
}

func (u weekDayUpgrader) UpgradeSpec() *unikornv1.ApplicationBundleAutoUpgradeSpec {
	spec := &unikornv1.ApplicationBundleAutoUpgradeWeekDaySpec{}

	window := &unikornv1.ApplicationBundleAutoUpgradeWindowSpec{
		Start: u.start,
		End:   u.end,
	}

	switch u.day {
	case time.Sunday:
		spec.Sunday = window
	case time.Monday:
		spec.Monday = window
	case time.Tuesday:
		spec.Tuesday = window
	case time.Wednesday:
		spec.Wednesday = window
	case time.Thursday:
		spec.Thursday = window
	case time.Friday:
		spec.Friday = window
	case time.Saturday:
		spec.Saturday = window
	}

	return &unikornv1.ApplicationBundleAutoUpgradeSpec{
		WeekDay: spec,
	}
}

func logStats(t *testing.T, freqs map[int]int) {
	t.Helper()

	var mean float64

	for _, freq := range freqs {
		mean += float64(freq)
	}

	mean /= float64(len(freqs))

	var variance float64

	for _, freq := range freqs {
		diff := float64(freq) - mean

		variance += diff * diff
	}

	variance /= float64(len(freqs))

	stddev := math.Sqrt(variance)

	t.Log("mean", mean, "stddev", stddev, fmt.Sprintf("(%f%%)", stddev*100/mean))
}

func validToday(t *testing.T, window *util.TimeWindow) bool {
	t.Helper()

	now := time.Now()

	curr := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 1, 0, time.UTC)

	for range 24 {
		if curr.After(window.Start) && curr.Before(window.End) {
			return true
		}

		curr = curr.Add(time.Hour)
	}

	return false
}

func validYesterday(t *testing.T, window *util.TimeWindow) bool {
	t.Helper()

	now := time.Now()

	curr := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 1, 0, time.UTC)

	for range 24 {
		if curr.After(window.Start) && curr.Before(window.End) {
			return true
		}

		curr = curr.Add(time.Hour)
	}

	return false
}

// TestGenerateTimeWindow ensures time window generation spits out times when
// we expect it to.
func TestGenerateTimeWindow(t *testing.T) {
	t.Parallel()

	// Keep tabs on what days of the week and hours things get scheduled.
	daysOfWeek := map[int]int{}
	hoursOfDay := map[int]int{}

	// This is driven by RANDOM, so use enough iterations to be statistically
	// significant.
	for range samples {
		window := util.TimeWindowFromResource(t.Context(), &autoGeneratingUpgrader{})

		// We shouldn't be triggering fails unless we're in the office to
		// deal with them.
		if window.Start.Weekday() == time.Saturday || window.Start.Weekday() == time.Sunday {
			t.Fatal("start time is when we should be sipping pina coladas")
		}

		daysOfWeek[int(window.Start.Weekday())]++

		// We shouldn't be triggering fails during the working day.
		if window.Start.Hour() > 6 {
			t.Fatal("start time is during the working day")
		}

		hoursOfDay[window.Start.Hour()]++

		if window.End.Sub(window.Start) != time.Hour {
			t.Fatal("end time isn't one hour after the start time")
		}
	}

	// As a final check, ensure all the expected indices are there.
	for i := time.Monday; i != time.Saturday; i++ {
		if _, ok := daysOfWeek[int(i)]; !ok {
			t.Fatal("Nothing scheduled on day", i)
		}
	}

	t.Log("day of week statistics")
	logStats(t, daysOfWeek)

	for i := range 7 {
		if _, ok := hoursOfDay[i]; !ok {
			t.Fatal("Nothing scheduled at hour", i)
		}
	}

	t.Log("time of day statistics")
	logStats(t, hoursOfDay)
}

// TestWeekday tests that an upgrade window is returned when now is
// the requested window.
func TestWeekday(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	now := time.Now().UTC()
	upgrader := newWeekDayUpgrader(now.Weekday(), now.Hour(), now.Hour()+2)
	window := util.TimeWindowFromResource(ctx, upgrader)

	if !validToday(t, window) {
		t.Fatal("not valid at any point today")
	}
}

// TestWeekdayOverflow tests that an upgrade window is returned when now is
// still in yesterday's requested window.
func TestWeekdayOverflow(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	now := time.Now().UTC()
	upgrader := newWeekDayUpgrader((now.Weekday()+6)%7, now.Hour()+3, now.Hour()+2)
	window := util.TimeWindowFromResource(ctx, upgrader)

	if !validToday(t, window) && !validYesterday(t, window) {
		t.Fatal("not valid at any point today")
	}
}
