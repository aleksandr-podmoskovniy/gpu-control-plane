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

package health

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
)

func TestFilterHealthyHandler(t *testing.T) {
	st := state.New("node-1")
	st.SetReady([]gpuv1alpha1.PhysicalGPU{
		sampleGPUWithHealth("healthy", metav1.ConditionTrue),
		sampleGPUWithHealth("unhealthy", metav1.ConditionFalse),
		{
			ObjectMeta: metav1.ObjectMeta{Name: "no-condition"},
		},
	})

	h := NewFilterHealthyHandler()
	if err := h.Handle(context.Background(), st); err != nil {
		t.Fatalf("handle: %v", err)
	}

	ready := st.Ready()
	if len(ready) != 1 {
		t.Fatalf("expected 1 healthy gpu, got %d", len(ready))
	}
	if ready[0].Name != "healthy" {
		t.Fatalf("unexpected ready gpu: %s", ready[0].Name)
	}
}

func sampleGPUWithHealth(name string, status metav1.ConditionStatus) gpuv1alpha1.PhysicalGPU {
	return gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: gpuv1alpha1.PhysicalGPUStatus{
			Conditions: []metav1.Condition{
				{
					Type:   handler.HardwareHealthyType,
					Status: status,
				},
			},
		},
	}
}
