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

package inventory

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestComputeCapabilityEqual(t *testing.T) {
	if !computeCapabilityEqual(nil, nil) {
		t.Fatal("expected nil capabilities to be equal")
	}
	left := &gpuv1alpha1.GPUComputeCapability{Major: 8, Minor: 6}
	if computeCapabilityEqual(left, nil) {
		t.Fatal("expected mismatch when one capability is nil")
	}
	right := &gpuv1alpha1.GPUComputeCapability{Major: 8, Minor: 6}
	if !computeCapabilityEqual(left, right) {
		t.Fatal("expected capabilities with same values to be equal")
	}
	right.Minor = 5
	if computeCapabilityEqual(left, right) {
		t.Fatal("expected capabilities with different minor to be different")
	}
}

func TestStringSlicesEqual(t *testing.T) {
	if !stringSlicesEqual(nil, nil) {
		t.Fatal("expected two nil slices to be equal")
	}
	if stringSlicesEqual([]string{"a"}, []string{"b"}) {
		t.Fatal("expected different slices to be unequal")
	}
	if stringSlicesEqual([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("expected slices with different lengths to be unequal")
	}
	if !stringSlicesEqual([]string{"a", "b"}, []string{"a", "b"}) {
		t.Fatal("expected same content slices to be equal")
	}
}

func TestSetStatusCondition(t *testing.T) {
	var conditions []metav1.Condition
	changed := setStatusCondition(&conditions, metav1.Condition{
		Type:               conditionManagedDisabled,
		Status:             metav1.ConditionFalse,
		Reason:             reasonNodeManagedEnabled,
		Message:            "initial",
		ObservedGeneration: 1,
	})
	if !changed {
		t.Fatal("expected condition set to report change when newly added")
	}

	changed = setStatusCondition(&conditions, metav1.Condition{
		Type:               conditionManagedDisabled,
		Status:             metav1.ConditionFalse,
		Reason:             reasonNodeManagedEnabled,
		Message:            "initial",
		ObservedGeneration: 2,
	})
	if changed {
		t.Fatal("expected condition update with same state to be no-op")
	}

	changed = setStatusCondition(&conditions, metav1.Condition{
		Type:               conditionManagedDisabled,
		Status:             metav1.ConditionTrue,
		Reason:             reasonNodeManagedDisabled,
		Message:            "updated",
		ObservedGeneration: 3,
	})
	if !changed {
		t.Fatal("expected condition update with new status to report change")
	}
}
