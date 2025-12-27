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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetConditionAddsAndUpdates(t *testing.T) {
	var conds []metav1.Condition

	t0 := time.Unix(1000, 0)
	builder := NewConditionBuilder(ConditionType("Ready")).
		Status(metav1.ConditionTrue).
		Reason(CommonReason("Initial")).
		Message("ok").
		Generation(1).
		LastTransitionTime(t0)

	SetCondition(builder, &conds)
	if len(conds) != 1 {
		t.Fatalf("expected one condition, got %d", len(conds))
	}
	if conds[0].LastTransitionTime.Time != t0 {
		t.Fatalf("expected lastTransitionTime preserved, got %s", conds[0].LastTransitionTime.Time)
	}

	t1 := time.Unix(2000, 0)
	builder = NewConditionBuilder(ConditionType("Ready")).
		Status(metav1.ConditionFalse).
		Reason(CommonReason("Initial")).
		Message("boom").
		Generation(2).
		LastTransitionTime(t1)
	SetCondition(builder, &conds)

	cond := conds[0]
	if cond.Status != metav1.ConditionFalse || cond.Reason != "Initial" || cond.Message != "boom" || cond.ObservedGeneration != 2 {
		t.Fatalf("unexpected condition update: %+v", cond)
	}
	if cond.LastTransitionTime.Time != t1 {
		t.Fatalf("expected lastTransitionTime to be set to t1, got %s", cond.LastTransitionTime.Time)
	}

	t2 := time.Unix(3000, 0)
	builder = NewConditionBuilder(ConditionType("Ready")).
		Status(metav1.ConditionFalse).
		Reason(CommonReason("Failed")).
		Message("still bad").
		Generation(3).
		LastTransitionTime(t2)
	SetCondition(builder, &conds)

	cond = conds[0]
	if cond.Reason != "Failed" || cond.Message != "still bad" || cond.ObservedGeneration != 3 {
		t.Fatalf("unexpected condition update after reason change: %+v", cond)
	}
	if cond.LastTransitionTime.Time != t2 {
		t.Fatalf("expected lastTransitionTime to be set to t2, got %s", cond.LastTransitionTime.Time)
	}
}

func TestConditionHelpers(t *testing.T) {
	conds := []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
		{Type: "Synced", Status: metav1.ConditionFalse},
	}

	if FindStatusCondition(conds, "Ready") == nil {
		t.Fatalf("expected Ready condition to exist")
	}
	if FindStatusCondition(conds, "Missing") != nil {
		t.Fatalf("did not expect Missing condition to exist")
	}

	if cond := FindStatusCondition(conds, "Synced"); cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("expected to find Synced condition, got %+v", cond)
	}
}

func TestNewConditionBuilderDefaults(t *testing.T) {
	cond := NewConditionBuilder(ConditionType("Ready")).Condition()
	if cond.Status != metav1.ConditionUnknown {
		t.Fatalf("expected default status Unknown, got %s", cond.Status)
	}
	if cond.Reason != ReasonUnknown.String() {
		t.Fatalf("expected default reason Unknown, got %s", cond.Reason)
	}
}
