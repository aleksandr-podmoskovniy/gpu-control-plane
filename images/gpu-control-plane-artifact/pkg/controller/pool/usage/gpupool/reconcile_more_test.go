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
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

type notFoundOnSecondGetClient struct {
	client.Client
	getCalls int
}

func (c *notFoundOnSecondGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	c.getCalls++
	if c.getCalls >= 2 {
		return apierrors.NewNotFound(schema.GroupResource{Group: "gpu.deckhouse.io", Resource: "gpupools"}, key.Name)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func TestPoolUsageReconcileSkipsWhenDisabled(t *testing.T) {
	store := moduleconfig.NewModuleConfigStore(moduleconfig.State{Enabled: false})
	r := NewReconciler(testr.New(t), config.ControllerConfig{Workers: 1}, store)
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "pool"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

func TestPoolUsageReconcileIgnoresNotFound(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	r := NewReconciler(testr.New(t), config.ControllerConfig{Workers: 1}, nil)
	r.client = cl
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "pool"}}); err != nil {
		t.Fatalf("expected notfound to be ignored, got %v", err)
	}
}

func TestPoolUsageReconcileNoopWhenUpToDate(t *testing.T) {
	scheme := newScheme(t)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns1"},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 10, Used: 2, Available: 8},
		},
	}

	resourceName := corev1.ResourceName("gpu.deckhouse.io/pool-a")
	podLabels := map[string]string{
		poolcommon.PoolNameKey:  "pool-a",
		poolcommon.PoolScopeKey: poolcommon.PoolScopeNamespaced,
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1", Labels: podLabels},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{resourceName: resource.MustParse("2")},
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cl := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUPool{}).
		WithObjects(pool, pod).
		Build()

	r := NewReconciler(testr.New(t), config.ControllerConfig{Workers: 1}, nil)
	r.client = cl
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pool)}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

func TestPoolUsageReconcileClampsAvailableToZeroWhenOverused(t *testing.T) {
	scheme := newScheme(t)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns1"},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}

	resourceName := corev1.ResourceName("gpu.deckhouse.io/pool-a")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			Labels: map[string]string{
				poolcommon.PoolNameKey:  pool.Name,
				poolcommon.PoolScopeKey: poolcommon.PoolScopeNamespaced,
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{resourceName: resource.MustParse("2")},
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cl := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUPool{}).
		WithObjects(pool, pod).
		Build()

	r := NewReconciler(testr.New(t), config.ControllerConfig{Workers: 1}, nil)
	r.client = cl

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pool)}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &v1alpha1.GPUPool{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(pool), got); err != nil {
		t.Fatalf("get pool: %v", err)
	}
	if got.Status.Capacity.Used != 2 {
		t.Fatalf("expected used=2, got %d", got.Status.Capacity.Used)
	}
	if got.Status.Capacity.Available != 0 {
		t.Fatalf("expected available=0, got %d", got.Status.Capacity.Available)
	}
}

func TestPoolUsageReconcileRetryGetNotFoundIsIgnored(t *testing.T) {
	scheme := newScheme(t)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns1"},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 10},
		},
	}

	cl := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUPool{}).
		WithObjects(pool).
		Build()

	r := NewReconciler(testr.New(t), config.ControllerConfig{Workers: 1}, nil)
	r.client = &notFoundOnSecondGetClient{Client: cl}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pool)}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

type listPodsErrorClient struct{ client.Client }

func (c listPodsErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*corev1.PodList); ok {
		return errors.New("list pods error")
	}
	return c.Client.List(ctx, list, opts...)
}

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

func TestPoolUsageReconcileReturnsListError(t *testing.T) {
	scheme := newScheme(t)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns1"},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()

	r := NewReconciler(testr.New(t), config.ControllerConfig{Workers: 1}, nil)
	r.client = listPodsErrorClient{Client: cl}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pool)}); err == nil {
		t.Fatalf("expected list error")
	}
}

func TestPoolUsageReconcileReturnsStatusPatchError(t *testing.T) {
	scheme := newScheme(t)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns1"},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}
	base := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUPool{}).
		WithObjects(pool).
		Build()

	r := NewReconciler(testr.New(t), config.ControllerConfig{Workers: 1}, nil)
	r.client = statusPatchErrorClient{Client: base, err: errors.New("patch error")}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pool)}); err == nil {
		t.Fatalf("expected status patch error")
	}
}
