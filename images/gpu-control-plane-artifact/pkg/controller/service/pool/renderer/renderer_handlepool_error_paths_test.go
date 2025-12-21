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

package renderer

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr/testr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestRendererHandlePoolConfigErrorsAndEarlyReturns(t *testing.T) {
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "ns"}}

	h := &RendererHandler{}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected client required error")
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	h = &RendererHandler{client: cl, cfg: RenderConfig{DevicePluginImage: "dp:tag"}}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected namespace not configured error")
	}

	h = &RendererHandler{client: cl, cfg: RenderConfig{Namespace: "ns"}}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected device-plugin image not configured error")
	}

	unsupported := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "ns"}, Spec: v1alpha1.GPUPoolSpec{Provider: "AMD"}}
	h = &RendererHandler{client: cl, cfg: RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"}}
	if _, err := h.HandlePool(context.Background(), unsupported); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "nvidia-device-plugin-alpha"}, &appsv1.DaemonSet{}); err == nil {
		t.Fatalf("expected no resources to be created for unsupported provider")
	}
}

func TestRendererHandlePoolCapacityZeroTriggersCleanupAndListError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "ns"},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 0}},
	}

	base := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build()
	h := &RendererHandler{client: base, cfg: RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"}, log: testr.New(t)}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("expected cleanup path to succeed, got %v", err)
	}

	h = &RendererHandler{client: listErrorClient{Client: base, err: errors.New("list error")}, cfg: RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"}}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected poolHasAssignedDevices list error")
	}
}

func TestRendererHandlePoolPropagatesReconcileAndCleanupErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "ns"},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}

	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	cl := &createNthErrorClient{Client: base, failOn: 1, err: errors.New("create error")}
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected reconcileDevicePlugin error")
	}

	h = &RendererHandler{client: base, cfg: RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: ""}, log: testr.New(t)}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected reconcileValidator error")
	}

	del := &deleteNthErrorClient{Client: base, failOn: 1, err: errors.New("delete error")}
	h = NewRendererHandler(testr.New(t), del, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected cleanupMIGResources error")
	}
}

func TestRendererHandlePoolMIGBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "ns"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb"}},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := &RendererHandler{
		log:    testr.New(t),
		client: cl,
		cfg: RenderConfig{
			Namespace:         "ns",
			DevicePluginImage: "dp:tag",
			ValidatorImage:    "val:tag",
			MIGManagerImage:   "",
		},
	}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}

	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "nvidia-mig-manager-alpha"}, &appsv1.DaemonSet{}); err == nil {
		t.Fatalf("expected MIG manager to be skipped without image configured")
	}

	fail := &createNthErrorClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), failOn: 4, err: errors.New("create error")}
	h = NewRendererHandler(testr.New(t), fail, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag", MIGManagerImage: "mig:tag"})
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected MIG manager reconcile error")
	}
}
