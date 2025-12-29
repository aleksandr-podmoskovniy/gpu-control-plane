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

func TestCapabilitiesHandlerSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	pgpu := &gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-0"},
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

	snapshot := &service.DeviceSnapshot{
		Capabilities: &gpuv1alpha1.GPUCapabilities{
			ProductName: "NVIDIA A30",
			Vendor:      gpuv1alpha1.VendorNvidia,
		},
		CurrentState: &gpuv1alpha1.GPUCurrentState{
			Nvidia: &gpuv1alpha1.NvidiaCurrentState{
				CUDAVersion: "13.0",
			},
		},
	}

	session := &fakeSession{snapshot: snapshot}
	reader := &fakeReader{session: session}
	tracker := &fakeTracker{shouldAttempt: true, recordFailure: true}

	h := NewCapabilitiesHandler(reader, service.NewPhysicalGPUService(client), tracker)

	st := state.New("node-1")
	st.SetReady([]gpuv1alpha1.PhysicalGPU{*pgpu})

	if err := h.Handle(context.Background(), st); err != nil {
		t.Fatalf("handle: %v", err)
	}

	updated := &gpuv1alpha1.PhysicalGPU{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-0"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}

	cond := meta.FindStatusCondition(updated.Status.Conditions, hardwareHealthyType)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected HardwareHealthy True, got %#v", cond)
	}

	if updated.Status.Capabilities == nil || updated.Status.Capabilities.ProductName != "NVIDIA A30" {
		t.Fatalf("capabilities not populated")
	}
	if updated.Status.CurrentState == nil || updated.Status.CurrentState.Nvidia == nil {
		t.Fatalf("current state missing")
	}
	if updated.Status.CurrentState.DriverType != gpuv1alpha1.DriverTypeNvidia {
		t.Fatalf("driver type overwritten: %v", updated.Status.CurrentState.DriverType)
	}
	if updated.Status.CurrentState.Nvidia.CUDAVersion != "13.0" {
		t.Fatalf("unexpected cuda version: %s", updated.Status.CurrentState.Nvidia.CUDAVersion)
	}
	if len(tracker.cleared) != 1 {
		t.Fatalf("expected tracker cleared once, got %d", len(tracker.cleared))
	}
	if !session.closed {
		t.Fatalf("expected session to be closed")
	}
}

func TestCapabilitiesHandlerOpenFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	pgpu := &gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-1"},
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
	tracker := &fakeTracker{shouldAttempt: true, recordFailure: true}
	h := NewCapabilitiesHandler(reader, service.NewPhysicalGPUService(client), tracker)

	st := state.New("node-1")
	st.SetReady([]gpuv1alpha1.PhysicalGPU{*pgpu})

	if err := h.Handle(context.Background(), st); err != nil {
		t.Fatalf("handle: %v", err)
	}

	updated := &gpuv1alpha1.PhysicalGPU{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-1"}, updated); err != nil {
		t.Fatalf("get: %v", err)
	}
	cond := meta.FindStatusCondition(updated.Status.Conditions, hardwareHealthyType)
	if cond == nil || cond.Status != metav1.ConditionUnknown || cond.Reason != reasonNVMLUnavailable {
		t.Fatalf("expected HardwareHealthy Unknown NVMLUnavailable, got %#v", cond)
	}
}

type fakeReader struct {
	session service.CapabilitiesSession
	err     error
}

func (f *fakeReader) Open() (service.CapabilitiesSession, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.session, nil
}

type fakeSession struct {
	snapshot *service.DeviceSnapshot
	err      error
	closed   bool
}

func (s *fakeSession) Close() {
	s.closed = true
}

func (s *fakeSession) ReadDevice(_ string) (*service.DeviceSnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshot, nil
}

type fakeTracker struct {
	shouldAttempt bool
	recordFailure bool
	cleared       []string
}

func (t *fakeTracker) ShouldAttempt(_ string) bool {
	return t.shouldAttempt
}

func (t *fakeTracker) RecordFailure(_ string) bool {
	return t.recordFailure
}

func (t *fakeTracker) Clear(name string) {
	t.cleared = append(t.cleared, name)
}
