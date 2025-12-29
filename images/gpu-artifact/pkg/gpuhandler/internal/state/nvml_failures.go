/*
Copyright 2025 Flant JSC

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

package state

import (
	"time"

	"k8s.io/utils/clock"
)

var (
	nvmlFailureGrace   = 30 * time.Second
	nvmlFailureBackoff = []time.Duration{5 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second, 5 * time.Minute}
)

// NVMLFailureTracker controls retry and grace logic for NVML failures.
type NVMLFailureTracker struct {
	clock    clock.Clock
	failures map[string]*nvmlFailureState
}

type nvmlFailureState struct {
	firstFailure time.Time
	nextAttempt  time.Time
	backoffIndex int
}

// NewNVMLFailureTracker constructs a tracker for NVML failures.
func NewNVMLFailureTracker(clk clock.Clock) *NVMLFailureTracker {
	if clk == nil {
		clk = clock.RealClock{}
	}
	return &NVMLFailureTracker{
		clock:    clk,
		failures: make(map[string]*nvmlFailureState),
	}
}

// ShouldAttempt returns true if a retry should be attempted now.
func (t *NVMLFailureTracker) ShouldAttempt(name string) bool {
	entry := t.failures[name]
	if entry == nil {
		return true
	}
	if entry.nextAttempt.IsZero() {
		return true
	}
	return !t.clock.Now().Before(entry.nextAttempt)
}

// RecordFailure updates retry state and returns true when grace elapsed.
func (t *NVMLFailureTracker) RecordFailure(name string) bool {
	now := t.clock.Now()
	entry := t.failures[name]
	if entry == nil {
		entry = &nvmlFailureState{firstFailure: now}
		t.failures[name] = entry
	}

	backoffIndex := entry.backoffIndex
	if backoffIndex >= len(nvmlFailureBackoff) {
		backoffIndex = len(nvmlFailureBackoff) - 1
	}
	entry.nextAttempt = now.Add(nvmlFailureBackoff[backoffIndex])
	entry.backoffIndex++

	return now.Sub(entry.firstFailure) >= nvmlFailureGrace
}

// Clear forgets failure state for a device.
func (t *NVMLFailureTracker) Clear(name string) {
	delete(t.failures, name)
}
