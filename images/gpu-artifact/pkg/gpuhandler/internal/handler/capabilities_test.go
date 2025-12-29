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

func TestCapabilitiesHandlerMultiGPU(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}

	pgpuA := &gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-a"},
		Status: gpuv1alpha1.PhysicalGPUStatus{
			PCIInfo: &gpuv1alpha1.PCIInfo{Address: "0000:02:00.0"},
			CurrentState: &gpuv1alpha1.GPUCurrentState{
				DriverType: gpuv1alpha1.DriverTypeNvidia,
			},
		},
	}
	pgpuB := &gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-b"},
		Status: gpuv1alpha1.PhysicalGPUStatus{
			PCIInfo: &gpuv1alpha1.PCIInfo{Address: "0000:03:00.0"},
			CurrentState: &gpuv1alpha1.GPUCurrentState{
				DriverType: gpuv1alpha1.DriverTypeNvidia,
			},
		},
	}
	meta.SetStatusCondition(&pgpuA.Status.Conditions, metav1.Condition{
		Type:   driverReadyType,
		Status: metav1.ConditionTrue,
	})
	meta.SetStatusCondition(&pgpuB.Status.Conditions, metav1.Condition{
		Type:   driverReadyType,
		Status: metav1.ConditionTrue,
	})

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gpuv1alpha1.PhysicalGPU{}).
		WithObjects(pgpuA, pgpuB).
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

	session := &fakeSession{
		snapshots: map[string]*service.DeviceSnapshot{
			"0000:02:00.0": snapshot,
		},
		errs: map[string]error{
			"0000:03:00.0": service.ErrNVMLQueryFailed,
		},
	}
	reader := &fakeReader{session: session}
	tracker := &fakeTracker{shouldAttempt: true, recordFailure: true}
	h := NewCapabilitiesHandler(reader, service.NewPhysicalGPUService(client), tracker)

	st := state.New("node-1")
	st.SetReady([]gpuv1alpha1.PhysicalGPU{*pgpuA, *pgpuB})

	if err := h.Handle(context.Background(), st); err != nil {
		t.Fatalf("handle: %v", err)
	}

	updatedA := &gpuv1alpha1.PhysicalGPU{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-a"}, updatedA); err != nil {
		t.Fatalf("get gpu-a: %v", err)
	}
	cond := meta.FindStatusCondition(updatedA.Status.Conditions, hardwareHealthyType)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("gpu-a expected HardwareHealthy True, got %#v", cond)
	}
	if updatedA.Status.Capabilities == nil || updatedA.Status.Capabilities.ProductName != "NVIDIA A30" {
		t.Fatalf("gpu-a capabilities not populated")
	}

	updatedB := &gpuv1alpha1.PhysicalGPU{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-b"}, updatedB); err != nil {
		t.Fatalf("get gpu-b: %v", err)
	}
	cond = meta.FindStatusCondition(updatedB.Status.Conditions, hardwareHealthyType)
	if cond == nil || cond.Status != metav1.ConditionUnknown || cond.Reason != reasonNVMLQueryFailed {
		t.Fatalf("gpu-b expected HardwareHealthy Unknown NVMLQueryFailed, got %#v", cond)
	}

	if !session.closed {
		t.Fatalf("expected session to be closed")
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
	snapshot  *service.DeviceSnapshot
	err       error
	snapshots map[string]*service.DeviceSnapshot
	errs      map[string]error
	closed    bool
}

func (s *fakeSession) Close() {
	s.closed = true
}

func (s *fakeSession) ReadDevice(pciAddress string) (*service.DeviceSnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.errs != nil {
		if err, ok := s.errs[pciAddress]; ok {
			return nil, err
		}
	}
	if s.snapshots != nil {
		if snapshot, ok := s.snapshots[pciAddress]; ok {
			return snapshot, nil
		}
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
