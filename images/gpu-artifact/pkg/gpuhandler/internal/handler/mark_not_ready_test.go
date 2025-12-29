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

package handler

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
)

func TestMarkNotReadyHandler(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	ready := &gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-ready"},
		Status: gpuv1alpha1.PhysicalGPUStatus{
			PCIInfo: &gpuv1alpha1.PCIInfo{Address: "0000:02:00.0"},
		},
	}
	meta.SetStatusCondition(&ready.Status.Conditions, metav1.Condition{
		Type:   driverReadyType,
		Status: metav1.ConditionTrue,
	})

	notReady := &gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-not-ready"},
		Status: gpuv1alpha1.PhysicalGPUStatus{
			PCIInfo: &gpuv1alpha1.PCIInfo{Address: "0000:03:00.0"},
		},
	}
	meta.SetStatusCondition(&notReady.Status.Conditions, metav1.Condition{
		Type:   driverReadyType,
		Status: metav1.ConditionFalse,
	})

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gpuv1alpha1.PhysicalGPU{}).
		WithObjects(ready, notReady).
		Build()

	tracker := &fakeTracker{}
	h := NewMarkNotReadyHandler(service.NewPhysicalGPUService(client), tracker)

	st := state.New("node-1")
	st.SetAll([]gpuv1alpha1.PhysicalGPU{*ready, *notReady})

	if err := h.Handle(context.Background(), st); err != nil {
		t.Fatalf("handle: %v", err)
	}

	updatedReady := &gpuv1alpha1.PhysicalGPU{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-ready"}, updatedReady); err != nil {
		t.Fatalf("get ready: %v", err)
	}
	cond := meta.FindStatusCondition(updatedReady.Status.Conditions, hardwareHealthyType)
	if cond != nil {
		t.Fatalf("expected HardwareHealthy unchanged for ready GPU, got %#v", cond)
	}

	updatedNotReady := &gpuv1alpha1.PhysicalGPU{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-not-ready"}, updatedNotReady); err != nil {
		t.Fatalf("get not ready: %v", err)
	}
	cond = meta.FindStatusCondition(updatedNotReady.Status.Conditions, hardwareHealthyType)
	if cond == nil || cond.Status != metav1.ConditionUnknown || cond.Reason != reasonDriverNotReady {
		t.Fatalf("expected HardwareHealthy Unknown DriverNotReady, got %#v", cond)
	}

	if len(tracker.cleared) != 1 || tracker.cleared[0] != "gpu-not-ready" {
		t.Fatalf("expected tracker cleared for gpu-not-ready, got %#v", tracker.cleared)
	}
}
