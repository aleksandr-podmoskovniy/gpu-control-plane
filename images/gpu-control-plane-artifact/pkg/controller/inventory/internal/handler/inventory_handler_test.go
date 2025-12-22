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

package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invservice "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/service"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

type stubState struct {
	node          *corev1.Node
	snapshot      invstate.NodeSnapshot
	approval      invstate.DeviceApprovalPolicy
	allowCleanup  bool
	orphanDevices map[string]struct{}
	orphanErr     error
}

func (s stubState) Node() *corev1.Node                            { return s.node }
func (s stubState) NodeFeature() *nfdv1alpha1.NodeFeature         { return nil }
func (s stubState) Snapshot() invstate.NodeSnapshot               { return s.snapshot }
func (s stubState) ApprovalPolicy() invstate.DeviceApprovalPolicy { return s.approval }
func (s stubState) AllowCleanup() bool                            { return s.allowCleanup }
func (s stubState) HasDevices() bool                              { return len(s.snapshot.Devices) > 0 }
func (s stubState) OrphanDevices(context.Context, client.Client) (map[string]struct{}, error) {
	return s.orphanDevices, s.orphanErr
}

type stubDeviceService struct {
	calls       int
	device      *v1alpha1.GPUDevice
	result      reconcile.Result
	err         error
	applyCalled bool
}

func (s *stubDeviceService) Reconcile(
	_ context.Context,
	_ *corev1.Node,
	snapshot invstate.DeviceSnapshot,
	_ map[string]string,
	_ bool,
	_ invstate.DeviceApprovalPolicy,
	applyDetection func(*v1alpha1.GPUDevice, invstate.DeviceSnapshot),
) (*v1alpha1.GPUDevice, reconcile.Result, error) {
	s.calls++
	device := s.device
	if device == nil {
		device = &v1alpha1.GPUDevice{}
	}
	if applyDetection != nil {
		s.applyCalled = true
		applyDetection(device, snapshot)
	}
	return device, s.result, s.err
}

type stubInventoryService struct {
	calls        int
	metricsCalls int
	err          error
}

func (s *stubInventoryService) Reconcile(context.Context, *corev1.Node, invstate.NodeSnapshot, []*v1alpha1.GPUDevice) error {
	s.calls++
	return s.err
}

func (s *stubInventoryService) UpdateDeviceMetrics(string, []*v1alpha1.GPUDevice) {
	s.metricsCalls++
}

type stubCleanupService struct {
	calls       int
	lastOrphans map[string]struct{}
	err         error
}

func (s *stubCleanupService) CleanupNode(context.Context, string) error { return nil }
func (s *stubCleanupService) DeleteInventory(context.Context, string) error {
	return nil
}
func (s *stubCleanupService) ClearMetrics(string) {}
func (s *stubCleanupService) RemoveOrphans(_ context.Context, _ *corev1.Node, orphans map[string]struct{}) error {
	s.calls++
	s.lastOrphans = orphans
	return s.err
}

type stubDetectionCollector struct {
	calls int
	err   error
}

func (s *stubDetectionCollector) Collect(context.Context, string) (invservice.NodeDetection, error) {
	s.calls++
	return invservice.NodeDetection{}, s.err
}

func TestInventoryHandlerSkipsWhenNoFeatureAndNoDevices(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}
	state := stubState{
		node:     node,
		snapshot: invstate.NodeSnapshot{FeatureDetected: false},
	}
	deviceSvc := &stubDeviceService{}
	inventorySvc := &stubInventoryService{}
	cleanupSvc := &stubCleanupService{}
	detectionSvc := &stubDetectionCollector{}

	handler := NewInventoryHandler(testr.New(t), nil, deviceSvc, inventorySvc, cleanupSvc, detectionSvc, nil)
	res, err := handler.Handle(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != (reconcile.Result{}) {
		t.Fatalf("expected empty result, got %+v", res)
	}
	if deviceSvc.calls != 0 || inventorySvc.calls != 0 || detectionSvc.calls != 0 {
		t.Fatalf("expected no services to be called, got device=%d inventory=%d detection=%d", deviceSvc.calls, inventorySvc.calls, detectionSvc.calls)
	}
}

func TestInventoryHandlerMergesRequeueResults(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-requeue"}}
	state := stubState{
		node: node,
		snapshot: invstate.NodeSnapshot{
			FeatureDetected: true,
			Managed:         true,
			Devices: []invstate.DeviceSnapshot{{
				Index:  "0",
				Vendor: "10de",
				Device: "2203",
				Class:  "0302",
			}},
		},
	}
	deviceSvc := &stubDeviceService{
		result: reconcile.Result{RequeueAfter: 2 * time.Second},
	}
	inventorySvc := &stubInventoryService{}
	handler := NewInventoryHandler(testr.New(t), nil, deviceSvc, inventorySvc, &stubCleanupService{}, &stubDetectionCollector{}, nil)

	res, err := handler.Handle(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RequeueAfter != 2*time.Second {
		t.Fatalf("expected requeueAfter=2s, got %+v", res)
	}
	if inventorySvc.calls != 1 || inventorySvc.metricsCalls != 1 {
		t.Fatalf("expected inventory service to be called once, got reconcile=%d metrics=%d", inventorySvc.calls, inventorySvc.metricsCalls)
	}
}

func TestInventoryHandlerReturnsRequeueOnInventoryConflict(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-conflict"}}
	state := stubState{
		node: node,
		snapshot: invstate.NodeSnapshot{
			FeatureDetected: true,
			Managed:         true,
			Devices: []invstate.DeviceSnapshot{{
				Index:  "0",
				Vendor: "10de",
				Device: "2203",
				Class:  "0302",
			}},
		},
	}
	deviceSvc := &stubDeviceService{}
	inventorySvc := &stubInventoryService{
		err: apierrors.NewConflict(schema.GroupResource{Group: "gpu.deckhouse.io", Resource: "gpunodestates"}, "node-conflict", errors.New("conflict")),
	}

	handler := NewInventoryHandler(testr.New(t), nil, deviceSvc, inventorySvc, &stubCleanupService{}, &stubDetectionCollector{}, nil)
	res, err := handler.Handle(context.Background(), state)
	if err != nil {
		t.Fatalf("expected conflict to be swallowed, got %v", err)
	}
	if !res.Requeue {
		t.Fatalf("expected requeue on conflict, got %+v", res)
	}
}

func TestInventoryHandlerCleansOrphansOnNodeDeletion(t *testing.T) {
	now := metav1.Now()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "node-delete",
			DeletionTimestamp: &now,
		},
	}
	state := stubState{
		node:         node,
		allowCleanup: true,
		orphanDevices: map[string]struct{}{
			"device-a": {},
			"device-b": {},
		},
		snapshot: invstate.NodeSnapshot{
			FeatureDetected: true,
			Managed:         true,
			Devices: []invstate.DeviceSnapshot{{
				Index:  "0",
				Vendor: "10de",
				Device: "2203",
				Class:  "0302",
			}},
		},
	}

	deviceSvc := &stubDeviceService{
		device: &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "device-a"}},
	}
	cleanupSvc := &stubCleanupService{}
	handler := NewInventoryHandler(testr.New(t), nil, deviceSvc, &stubInventoryService{}, cleanupSvc, &stubDetectionCollector{}, nil)

	if _, err := handler.Handle(context.Background(), state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanupSvc.calls != 1 {
		t.Fatalf("expected cleanup service to be called, got %d", cleanupSvc.calls)
	}
	if _, ok := cleanupSvc.lastOrphans["device-a"]; ok {
		t.Fatalf("expected reconciled device to be removed from orphan set")
	}
	if _, ok := cleanupSvc.lastOrphans["device-b"]; !ok {
		t.Fatalf("expected remaining orphan to be present")
	}
}

func TestInventoryHandlerCallsDetectionCollectorWhenDevicesPresent(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-detect"}}
	state := stubState{
		node: node,
		snapshot: invstate.NodeSnapshot{
			FeatureDetected: true,
			Managed:         true,
			Devices: []invstate.DeviceSnapshot{{
				Index:  "0",
				Vendor: "10de",
				Device: "2203",
				Class:  "0302",
			}},
		},
	}

	deviceSvc := &stubDeviceService{}
	detectionSvc := &stubDetectionCollector{}
	handler := NewInventoryHandler(testr.New(t), nil, deviceSvc, &stubInventoryService{}, &stubCleanupService{}, detectionSvc, nil)

	if _, err := handler.Handle(context.Background(), state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detectionSvc.calls != 1 {
		t.Fatalf("expected detection collector to be called once, got %d", detectionSvc.calls)
	}
	if !deviceSvc.applyCalled {
		t.Fatalf("expected applyDetection to be invoked by device service")
	}
}

func TestInventoryHandlerSkipsWhenNodeMissing(t *testing.T) {
	handler := NewInventoryHandler(testr.New(t), nil, &stubDeviceService{}, &stubInventoryService{}, &stubCleanupService{}, &stubDetectionCollector{}, nil)
	res, err := handler.Handle(context.Background(), stubState{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != (reconcile.Result{}) {
		t.Fatalf("expected empty result, got %+v", res)
	}
}

func TestInventoryHandlerOrphanDevicesError(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-orphan-error", UID: types.UID("node-orphan-error")}}
	state := stubState{
		node:         node,
		allowCleanup: true,
		orphanErr:    errors.New("orphan list failed"),
		snapshot: invstate.NodeSnapshot{
			FeatureDetected: true,
			Managed:         true,
			Devices: []invstate.DeviceSnapshot{{
				Index:  "0",
				Vendor: "10de",
				Device: "2203",
				Class:  "0302",
			}},
		},
	}

	handler := NewInventoryHandler(testr.New(t), nil, &stubDeviceService{}, &stubInventoryService{}, &stubCleanupService{}, &stubDetectionCollector{}, nil)
	if _, err := handler.Handle(context.Background(), state); err == nil {
		t.Fatalf("expected orphan list error")
	}
}
