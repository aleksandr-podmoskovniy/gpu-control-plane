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

package inventory

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
)

func TestFilterReadyHandler(t *testing.T) {
	st := state.New("node-1")
	st.SetAll([]gpuv1alpha1.PhysicalGPU{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ready"},
			Status: gpuv1alpha1.PhysicalGPUStatus{
				Conditions: []metav1.Condition{
					{
						Type:   handler.DriverReadyType,
						Status: metav1.ConditionTrue,
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "not-ready"},
			Status: gpuv1alpha1.PhysicalGPUStatus{
				Conditions: []metav1.Condition{
					{
						Type:   handler.DriverReadyType,
						Status: metav1.ConditionFalse,
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "no-condition"},
		},
	})

	h := NewFilterReadyHandler()
	if err := h.Handle(context.Background(), st); err != nil {
		t.Fatalf("handle: %v", err)
	}

	ready := st.Ready()
	if len(ready) != 1 {
		t.Fatalf("expected 1 ready gpu, got %d", len(ready))
	}
	if ready[0].Name != "ready" {
		t.Fatalf("unexpected ready gpu: %s", ready[0].Name)
	}
}
