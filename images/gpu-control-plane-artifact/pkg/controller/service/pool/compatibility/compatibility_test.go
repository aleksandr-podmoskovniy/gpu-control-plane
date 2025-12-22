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

package compatibility

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestCompatibilityCheckHandlerSupported(t *testing.T) {
	h := NewCompatibilityCheckHandler()
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
		Spec: v1alpha1.GPUPoolSpec{
			Provider: "Nvidia",
			Backend:  "DevicePlugin",
		},
	}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("handle pool: %v", err)
	}
	cond := getCondition(pool.Status.Conditions, conditionSupported)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected supported condition true, got %+v", cond)
	}
}

func TestCompatibilityCheckHandlerUnsupportedProvider(t *testing.T) {
	h := NewCompatibilityCheckHandler()
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
		Spec: v1alpha1.GPUPoolSpec{
			Provider: "AMD",
			Backend:  "DevicePlugin",
		},
	}
	_, _ = h.HandlePool(context.Background(), pool)
	cond := getCondition(pool.Status.Conditions, conditionSupported)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "UnsupportedProvider" {
		t.Fatalf("expected unsupported provider condition, got %+v", cond)
	}
}

func TestCompatibilityCheckHandlerUnsupportedBackend(t *testing.T) {
	h := NewCompatibilityCheckHandler()
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
		Spec: v1alpha1.GPUPoolSpec{
			Provider: "Nvidia",
			Backend:  "DRA",
		},
	}
	_, _ = h.HandlePool(context.Background(), pool)
	cond := getCondition(pool.Status.Conditions, conditionSupported)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "UnsupportedBackend" {
		t.Fatalf("expected unsupported backend condition, got %+v", cond)
	}
}

func getCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}

func TestCompatibilityCheckHandlerName(t *testing.T) {
	if NewCompatibilityCheckHandler().Name() == "" {
		t.Fatalf("expected handler name")
	}
}
