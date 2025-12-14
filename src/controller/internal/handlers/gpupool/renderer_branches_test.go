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

package gpupool

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestReconcileDevicePluginErrorPaths(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns", UID: "uid"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}

	t.Run("configmap error", func(t *testing.T) {
		base := fake.NewClientBuilder().WithScheme(scheme).Build()
		createErr := apierrors.NewBadRequest("cm create failed")
		cl := &selectiveCreateErrorClient{
			Client: base,
			err:    createErr,
			match: func(obj client.Object) bool {
				_, ok := obj.(*corev1.ConfigMap)
				return ok
			},
		}
		h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
		if err := h.reconcileDevicePlugin(context.Background(), pool); err == nil || !apierrors.IsBadRequest(err) {
			t.Fatalf("expected configmap error, got %v", err)
		}
	})

	t.Run("daemonset error", func(t *testing.T) {
		base := fake.NewClientBuilder().WithScheme(scheme).Build()
		createErr := apierrors.NewBadRequest("ds create failed")
		cl := &selectiveCreateErrorClient{
			Client: base,
			err:    createErr,
			match: func(obj client.Object) bool {
				_, ok := obj.(*appsv1.DaemonSet)
				return ok && strings.HasPrefix(obj.GetName(), "nvidia-device-plugin-")
			},
		}
		h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
		if err := h.reconcileDevicePlugin(context.Background(), pool); err == nil || !apierrors.IsBadRequest(err) {
			t.Fatalf("expected daemonset error, got %v", err)
		}
	})
}

func TestReconcileValidatorCreateOrUpdateError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", UID: "uid"}}

	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	createErr := apierrors.NewBadRequest("validator create failed")
	cl := &selectiveCreateErrorClient{
		Client: base,
		err:    createErr,
		match: func(obj client.Object) bool {
			_, ok := obj.(*appsv1.DaemonSet)
			return ok && strings.HasPrefix(obj.GetName(), "nvidia-operator-validator-")
		},
	}
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
	if err := h.reconcileValidator(context.Background(), pool); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected validator create error, got %v", err)
	}
}

func TestReconcileMIGManagerErrorPaths(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", UID: "uid"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb"}},
	}

	failures := []string{
		"nvidia-mig-manager-pool-config",
		"nvidia-mig-manager-pool-scripts",
		"nvidia-mig-manager-pool-gpu-clients",
		"nvidia-mig-manager-pool",
	}

	for _, failName := range failures {
		t.Run("fail="+failName, func(t *testing.T) {
			base := fake.NewClientBuilder().WithScheme(scheme).Build()
			createErr := apierrors.NewBadRequest("create failed")
			cl := &selectiveCreateErrorClient{
				Client: base,
				err:    createErr,
				match: func(obj client.Object) bool {
					return obj.GetName() == failName
				},
			}
			h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag", MIGManagerImage: "mig:tag"})
			if err := h.reconcileMIGManager(context.Background(), pool); err == nil || !apierrors.IsBadRequest(err) {
				t.Fatalf("expected create error, got %v", err)
			}
		})
	}
}

func TestCleanupMIGResourcesConfigMapDeleteError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	deleteErr := errors.New("delete failed")
	h := NewRendererHandler(testr.New(t), &selectiveDeleteErrorClient{
		Client: base,
		err:    deleteErr,
		match: func(obj client.Object) bool {
			_, ok := obj.(*corev1.ConfigMap)
			return ok && strings.HasSuffix(obj.GetName(), "-scripts")
		},
	}, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})

	if err := h.cleanupMIGResources(context.Background(), "pool"); err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("expected delete error, got %v", err)
	}
}

func TestCleanupPoolResourcesErrorPaths(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	deleteErr := errors.New("delete failed")

	tests := []struct {
		name  string
		match func(client.Object) bool
	}{
		{
			name: "device-plugin-daemonset",
			match: func(obj client.Object) bool {
				_, ok := obj.(*appsv1.DaemonSet)
				return ok && strings.HasPrefix(obj.GetName(), "nvidia-device-plugin-")
			},
		},
		{
			name: "device-plugin-configmap",
			match: func(obj client.Object) bool {
				_, ok := obj.(*corev1.ConfigMap)
				return ok && strings.Contains(obj.GetName(), "nvidia-device-plugin-") && strings.HasSuffix(obj.GetName(), "-config")
			},
		},
		{
			name: "mig-resources",
			match: func(obj client.Object) bool {
				return strings.HasPrefix(obj.GetName(), "nvidia-mig-manager-")
			},
		},
		{
			name: "validator-daemonset",
			match: func(obj client.Object) bool {
				_, ok := obj.(*appsv1.DaemonSet)
				return ok && strings.HasPrefix(obj.GetName(), "nvidia-operator-validator-")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := fake.NewClientBuilder().WithScheme(scheme).Build()
			h := NewRendererHandler(testr.New(t), &selectiveDeleteErrorClient{Client: base, err: deleteErr, match: tt.match}, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
			if err := h.cleanupPoolResources(context.Background(), "pool"); err == nil || !strings.Contains(err.Error(), "delete failed") {
				t.Fatalf("expected delete error, got %v", err)
			}
		})
	}
}

func TestAssignedDevicePatternsAndPoolHasAssignedDevicesBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	handler := &RendererHandler{}
	if got, err := handler.poolHasAssignedDevices(context.Background(), nil); err != nil || got {
		t.Fatalf("expected nil pool to return false,nil, got %t,%v", got, err)
	}
	if got := handler.assignedDevicePatterns(context.Background(), nil); got != nil {
		t.Fatalf("expected nil patterns for nil pool, got %#v", got)
	}

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}
	handler = NewRendererHandler(testr.New(t), nil, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
	if got, err := handler.poolHasAssignedDevices(context.Background(), pool); err != nil || got {
		t.Fatalf("expected nil client to return false,nil, got %t,%v", got, err)
	}

	base := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build()
	handler.client = &failingListClient{Client: base}
	if got := handler.assignedDevicePatterns(context.Background(), pool); got != nil {
		t.Fatalf("expected nil patterns on list error, got %#v", got)
	}

	ignored := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ignored",
			Labels: map[string]string{deviceIgnoreKey: "true"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
			State:   v1alpha1.GPUDeviceStateAssigned,
			Hardware: v1alpha1.GPUDeviceHardware{
				UUID: "GPU-IGNORED",
			},
		},
	}
	mismatch := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "mismatch"},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "other"},
			State:   v1alpha1.GPUDeviceStateAssigned,
			Hardware: v1alpha1.GPUDeviceHardware{
				UUID: "GPU-MISMATCH",
			},
		},
	}
	noUUID := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "nouuid"},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
			State:   v1alpha1.GPUDeviceStateAssigned,
		},
	}
	readyState := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "ready"},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
			State:   v1alpha1.GPUDeviceStateReady,
			Hardware: v1alpha1.GPUDeviceHardware{
				UUID: "GPU-READY",
			},
		},
	}
	dupA := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dup-a"},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
			State:   v1alpha1.GPUDeviceStatePendingAssignment,
			Hardware: v1alpha1.GPUDeviceHardware{
				UUID: "GPU-OK",
			},
		},
	}
	dupB := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dup-b"},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
			State:   v1alpha1.GPUDeviceStateReserved,
			Hardware: v1alpha1.GPUDeviceHardware{
				UUID: "GPU-OK",
			},
		},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).
		WithObjects(ignored, mismatch, noUUID, readyState, dupA, dupB).
		Build()

	handler.client = cl
	patterns := handler.assignedDevicePatterns(context.Background(), pool)
	if len(patterns) != 1 || patterns[0] != "GPU-OK" {
		t.Fatalf("unexpected patterns: %#v", patterns)
	}

	has, err := handler.poolHasAssignedDevices(context.Background(), pool)
	if err != nil || !has {
		t.Fatalf("expected assigned devices present, got has=%t err=%v", has, err)
	}
}

func TestPoolHasAssignedDevicesSkipsIgnoredAndMismatched(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}
	ignored := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "a-ignored",
			Labels: map[string]string{deviceIgnoreKey: "true"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	mismatch := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "b-mismatch"},
		Status: v1alpha1.GPUDeviceStatus{
			PoolRef: &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "other"},
		},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(ignored, mismatch).Build()
	h := &RendererHandler{client: cl}

	has, err := h.poolHasAssignedDevices(context.Background(), pool)
	if err != nil || has {
		t.Fatalf("expected ignored/mismatched devices to yield false,nil, got has=%t err=%v", has, err)
	}
}

func TestHandlePoolPropagatesReconcileDevicePluginError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	createErr := apierrors.NewBadRequest("create failed")
	base := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build()
	cl := &selectiveCreateErrorClient{
		Client: base,
		err:    createErr,
		match: func(obj client.Object) bool {
			_, ok := obj.(*corev1.ConfigMap)
			return ok && strings.HasPrefix(obj.GetName(), "nvidia-device-plugin-")
		},
	}
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", UID: "uid"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}

	if _, err := h.HandlePool(context.Background(), pool); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected device-plugin reconcile error, got %v", err)
	}
}
