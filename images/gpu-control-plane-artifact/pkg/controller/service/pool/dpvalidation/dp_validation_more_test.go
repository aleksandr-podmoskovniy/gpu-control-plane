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

package dpvalidation

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

type statusPatchErrorWriter struct {
	client.StatusWriter
	err error
}

func (w statusPatchErrorWriter) Patch(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
	return w.err
}

type statusPatchErrorClient struct {
	client.Client
	err error
}

func (c statusPatchErrorClient) Status() client.StatusWriter {
	return statusPatchErrorWriter{StatusWriter: c.Client.Status(), err: c.err}
}

type dpListErrorClient struct {
	client.Client
	err error
}

func (c dpListErrorClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return c.err
}

func TestNewDPValidationHandlerNamespaceFromEnv(t *testing.T) {
	prev := os.Getenv("POD_NAMESPACE")
	t.Cleanup(func() { _ = os.Setenv("POD_NAMESPACE", prev) })
	_ = os.Setenv("POD_NAMESPACE", "custom-ns")

	h := NewDPValidationHandler(testr.New(t), nil)
	if h.ns != "custom-ns" {
		t.Fatalf("unexpected namespace: %q", h.ns)
	}
}

func TestNewDPValidationHandlerNamespaceDefault(t *testing.T) {
	prev := os.Getenv("POD_NAMESPACE")
	t.Cleanup(func() { _ = os.Setenv("POD_NAMESPACE", prev) })
	_ = os.Unsetenv("POD_NAMESPACE")

	h := NewDPValidationHandler(testr.New(t), nil)
	if h.ns != "d8-gpu-control-plane" {
		t.Fatalf("unexpected default namespace: %q", h.ns)
	}
}

func TestNewDPValidationHandlerDefaultPodListReturnsError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	cl := dpListErrorClient{Client: base, err: errors.New("list error")}

	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}); err == nil {
		t.Fatalf("expected default podList to surface list error")
	}
}

func TestDPValidationHandlePoolNoopWithoutClient(t *testing.T) {
	h := NewDPValidationHandler(testr.New(t), nil)
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDPValidationHandlePoolSkipsPodWithoutNodeName(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator",
			Namespace: "ns",
			Labels: map[string]string{
				"app":  "nvidia-operator-validator",
				"pool": "pool",
			},
		},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithObjects(pod).
		Build()

	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
}

func TestDPValidationHandlePoolClusterAssignmentAndPodListBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{poolcommon.ClusterAssignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			State:    v1alpha1.GPUDeviceStatePendingAssignment,
			NodeName: "node1",
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator",
			Namespace: "ns",
			Labels: map[string]string{
				"app":  "nvidia-operator-validator",
				"pool": "pool",
			},
		},
		Spec: corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}

	cl := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device, pod).
		Build()

	h := NewDPValidationHandler(testr.New(t), cl)
	h.ns = "ns"
	h.podList = func(context.Context, client.Client, ...client.ListOption) (*corev1.PodList, error) {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "pods"}, "x")
	}

	clusterPool := &v1alpha1.GPUPool{TypeMeta: metav1.TypeMeta{Kind: "ClusterGPUPool"}, ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	if _, err := h.HandlePool(context.Background(), clusterPool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}

	h.podList = func(context.Context, client.Client, ...client.ListOption) (*corev1.PodList, error) {
		return nil, errors.New("boom")
	}
	if _, err := h.HandlePool(context.Background(), clusterPool); err == nil {
		t.Fatalf("expected pod list error")
	}
}

func TestDPValidationHandlePoolReturnsDeviceListErrorAndPatchError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dev1",
			Annotations: map[string]string{poolcommon.NamespacedAssignmentAnnotation: "pool"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			State:    v1alpha1.GPUDeviceStatePendingAssignment,
			NodeName: "node1",
		},
	}

	// Missing indexes for device list should return an error.
	noIndexClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device).
		Build()
	h := NewDPValidationHandler(testr.New(t), noIndexClient)
	h.ns = "ns"
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}); err == nil {
		t.Fatalf("expected device list error without indexes")
	}

	// Patch error must be returned.
	okClient := withPoolDeviceIndexes(fake.NewClientBuilder().
		WithScheme(scheme)).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(device).
		Build()
	h = NewDPValidationHandler(testr.New(t), statusPatchErrorClient{Client: okClient, err: errors.New("patch error")})
	h.ns = "ns"
	h.podList = func(context.Context, client.Client, ...client.ListOption) (*corev1.PodList, error) {
		return &corev1.PodList{Items: []corev1.Pod{{
			Spec:   corev1.PodSpec{NodeName: "node1"},
			Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
		}}}, nil
	}
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}); err == nil {
		t.Fatalf("expected patch error")
	}
}

func TestDPValidationIsPodReady(t *testing.T) {
	if isPodReady(nil) {
		t.Fatalf("nil pod must not be ready")
	}

	if isPodReady(&corev1.Pod{}) {
		t.Fatalf("pod without ready condition must not be ready")
	}

	if !isPodReady(&corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}) {
		t.Fatalf("expected ready condition to be detected")
	}
}
