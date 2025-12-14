// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package conditions

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilderSetPreservesTransitionTime(t *testing.T) {
	var conds []metav1.Condition
	b := New(&conds)

	initial := metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Initial",
		Message: "ok",
	}
	b.Set(initial)

	if len(conds) != 1 {
		t.Fatalf("expected one condition, got %d", len(conds))
	}
	firstTransition := conds[0].LastTransitionTime
	if firstTransition.IsZero() {
		t.Fatalf("expected transition time to be set")
	}

	// Same status keeps transition time.
	updated := metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "StillReady",
		Message: "still ok",
	}
	b.Set(updated)
	if conds[0].LastTransitionTime != firstTransition {
		t.Fatalf("expected transition time preserved, got %s -> %s", firstTransition, conds[0].LastTransitionTime)
	}
	if conds[0].Reason != "StillReady" || conds[0].Message != "still ok" {
		t.Fatalf("expected condition replaced with new content, got %+v", conds[0])
	}

	// Status change updates transition time.
	updated.Status = metav1.ConditionFalse
	b.Set(updated)
	if !conds[0].LastTransitionTime.After(firstTransition.Time) {
		t.Fatalf("expected transition time to advance on status change")
	}
}

func TestBuilderFind(t *testing.T) {
	conds := []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
		{Type: "Synced", Status: metav1.ConditionFalse},
	}
	b := New(&conds)

	found := b.Find("Synced")
	if found == nil || found.Status != metav1.ConditionFalse {
		t.Fatalf("expected to find Synced condition, got %+v", found)
	}
	if b.Find("Missing") != nil {
		t.Fatalf("expected nil for missing condition")
	}
}

func TestBuilderNilTargetIsNoOp(t *testing.T) {
	b := New(nil)
	b.Set(metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue})
	if b.Find("Ready") != nil {
		t.Fatalf("expected nil result for nil target")
	}
}

func TestBuilderSetAppendsWhenTypeNotFound(t *testing.T) {
	conds := []metav1.Condition{
		{Type: "Other", Status: metav1.ConditionTrue},
	}
	b := New(&conds)
	b.Set(metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue})

	if len(conds) != 2 {
		t.Fatalf("expected condition to be appended, got %#v", conds)
	}
	if conds[0].Type != "Other" || conds[1].Type != "Ready" {
		t.Fatalf("unexpected conditions order/content: %#v", conds)
	}
	if conds[1].LastTransitionTime.IsZero() {
		t.Fatalf("expected lastTransitionTime to be set")
	}
}
