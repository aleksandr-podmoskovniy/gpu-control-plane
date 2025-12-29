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
	"testing"
	"time"

	clocktesting "k8s.io/utils/clock/testing"
)

func TestNVMLFailureTracker(t *testing.T) {
	clk := clocktesting.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	tracker := NewNVMLFailureTracker(clk)

	if !tracker.ShouldAttempt("gpu-0") {
		t.Fatalf("expected first attempt to be allowed")
	}
	if tracker.RecordFailure("gpu-0") {
		t.Fatalf("expected grace period to be active")
	}
	if tracker.ShouldAttempt("gpu-0") {
		t.Fatalf("expected backoff to block immediate retry")
	}

	clk.Step(5 * time.Second)
	if !tracker.ShouldAttempt("gpu-0") {
		t.Fatalf("expected backoff to allow retry after 5s")
	}

	clk.Step(31 * time.Second)
	if !tracker.RecordFailure("gpu-0") {
		t.Fatalf("expected grace period to elapse")
	}

	tracker.Clear("gpu-0")
	if !tracker.ShouldAttempt("gpu-0") {
		t.Fatalf("expected cleared tracker to allow retry")
	}
}
