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
	"fmt"
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

func TestCapabilitiesHandlerSkipsNonNvidiaDriver(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	pgpu := &gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-2"},
		Status: gpuv1alpha1.PhysicalGPUStatus{
			CurrentState: &gpuv1alpha1.GPUCurrentState{
				DriverType: gpuv1alpha1.DriverTypeVFIO,
			},
		},
	}
	meta.SetStatusCondition(&pgpu.Status.Conditions, metav1.Condition{
		Type:   driverReadyType,
		Status: metav1.ConditionTrue,
	})

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gpuv1alpha1.PhysicalGPU{}).
		WithObjects(pgpu).
		Build()

	reader := &fakeReader{session: &fakeSession{}}
	tracker := &fakeTracker{shouldAttempt: true, recordFailure: true}
	h := NewCapabilitiesHandler(reader, service.NewPhysicalGPUService(client), tracker, nil)

	st := state.New("node-1")
	st.SetReady([]gpuv1alpha1.PhysicalGPU{*pgpu})

	if err := h.Handle(context.Background(), st); err != nil {
		t.Fatalf("handle: %v", err)
	}

	updated := &gpuv1alpha1.PhysicalGPU{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-2"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}

	cond := meta.FindStatusCondition(updated.Status.Conditions, hardwareHealthyType)
	if cond == nil || cond.Status != metav1.ConditionUnknown || cond.Reason != reasonDriverTypeNotNvidia {
		t.Fatalf("expected HardwareHealthy Unknown DriverTypeNotNvidia, got %#v", cond)
	}
	if len(tracker.cleared) != 1 {
		t.Fatalf("expected tracker cleared once, got %d", len(tracker.cleared))
	}
}

func TestCapabilitiesHandlerSkipsWhenShouldAttemptFalse(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	pgpu := &gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-3"},
		Status: gpuv1alpha1.PhysicalGPUStatus{
			PCIInfo: &gpuv1alpha1.PCIInfo{Address: "0000:02:00.0"},
			CurrentState: &gpuv1alpha1.GPUCurrentState{
				DriverType: gpuv1alpha1.DriverTypeNvidia,
			},
		},
	}
	meta.SetStatusCondition(&pgpu.Status.Conditions, metav1.Condition{
		Type:   driverReadyType,
		Status: metav1.ConditionTrue,
	})

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gpuv1alpha1.PhysicalGPU{}).
		WithObjects(pgpu).
		Build()

	reader := &fakeReader{session: &fakeSession{}}
	tracker := &fakeTracker{shouldAttempt: false, recordFailure: true}
	h := NewCapabilitiesHandler(reader, service.NewPhysicalGPUService(client), tracker, nil)

	st := state.New("node-1")
	st.SetReady([]gpuv1alpha1.PhysicalGPU{*pgpu})

	if err := h.Handle(context.Background(), st); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(tracker.cleared) != 0 {
		t.Fatalf("expected tracker not cleared, got %d", len(tracker.cleared))
	}
}

func TestCapabilitiesHandlerRecordFailureFalse(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	pgpu := &gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-4"},
		Status: gpuv1alpha1.PhysicalGPUStatus{
			PCIInfo: &gpuv1alpha1.PCIInfo{Address: "0000:02:00.0"},
			CurrentState: &gpuv1alpha1.GPUCurrentState{
				DriverType: gpuv1alpha1.DriverTypeNvidia,
			},
		},
	}
	meta.SetStatusCondition(&pgpu.Status.Conditions, metav1.Condition{
		Type:   hardwareHealthyType,
		Status: metav1.ConditionTrue,
	})

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gpuv1alpha1.PhysicalGPU{}).
		WithObjects(pgpu).
		Build()

	reader := &fakeReader{err: fmt.Errorf("init: %w", service.ErrNVMLUnavailable)}
	tracker := &fakeTracker{shouldAttempt: true, recordFailure: false}
	h := NewCapabilitiesHandler(reader, service.NewPhysicalGPUService(client), tracker, nil)

	st := state.New("node-1")
	st.SetReady([]gpuv1alpha1.PhysicalGPU{*pgpu})

	if err := h.Handle(context.Background(), st); err != nil {
		t.Fatalf("handle: %v", err)
	}

	updated := &gpuv1alpha1.PhysicalGPU{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-4"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}

	cond := meta.FindStatusCondition(updated.Status.Conditions, hardwareHealthyType)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected HardwareHealthy to remain True, got %#v", cond)
	}
}
