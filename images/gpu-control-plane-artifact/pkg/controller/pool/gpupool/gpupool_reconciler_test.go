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
	"time"

	"github.com/go-logr/logr/testr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	gpstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/pool/gpupool/internal/state"
)

type failingClient struct {
	client.Client
	err error
}

func (f *failingClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return f.err
}

type stubHandler struct {
	name   string
	result reconcile.Result
	err    error
	calls  int
}

func (s *stubHandler) Name() string { return s.name }
func (s *stubHandler) Handle(_ context.Context, st gpstate.PoolState) (reconcile.Result, error) {
	s.calls++
	if st != nil && st.Pool() != nil {
		st.Pool().Status.Capacity.Total++
	}
	return s.result, s.err
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func TestNewNormalisesWorkers(t *testing.T) {
	rec := NewReconciler(testr.New(t), config.ControllerConfig{Workers: 0}, nil, nil)
	if rec.cfg.Workers != 1 {
		t.Fatalf("expected workers defaulted to 1, got %d", rec.cfg.Workers)
	}
}

func TestReconcileAggregatesResultsAndPersistsStatus(t *testing.T) {
	scheme := newScheme(t)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}
	cl := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pool).
		WithStatusSubresource(pool).
		Build()

	handlerA := &stubHandler{name: "a", result: reconcile.Result{Requeue: true}}
	handlerB := &stubHandler{name: "b", result: reconcile.Result{RequeueAfter: time.Second}}

	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, []Handler{handlerA, handlerB})
	rec.client = cl

	res, err := rec.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "ns", Name: "pool"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("unexpected aggregate result: %+v", res)
	}
	if handlerA.calls != 1 || handlerB.calls != 1 {
		t.Fatalf("expected handlers invoked once, got %d/%d", handlerA.calls, handlerB.calls)
	}

	updated := &v1alpha1.GPUPool{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "pool"}, updated); err != nil {
		t.Fatalf("get updated pool: %v", err)
	}
	if updated.Status.Capacity.Total != 2 {
		t.Fatalf("expected status updates to be persisted, got %d", updated.Status.Capacity.Total)
	}
}

func TestReconcileHandlerError(t *testing.T) {
	scheme := newScheme(t)
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).WithStatusSubresource(pool).Build()

	handler := &stubHandler{name: "boom", err: errors.New("handler fail")}
	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, []Handler{handler})
	rec.client = cl

	if _, err := rec.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "ns", Name: "pool"}}); err == nil {
		t.Fatalf("expected handler error")
	}
	if handler.calls != 1 {
		t.Fatalf("expected handler called once, got %d", handler.calls)
	}
}

func TestReconcileNotFound(t *testing.T) {
	scheme := newScheme(t)
	cl := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = cl

	if _, err := rec.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "ns", Name: "missing"}}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestReconcileGetError(t *testing.T) {
	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingClient{err: errors.New("get fail")}

	if _, err := rec.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "ns", Name: "pool"}}); err == nil {
		t.Fatalf("expected get error")
	}
}

func TestReconcileWrapsAPIError(t *testing.T) {
	rec := NewReconciler(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingClient{err: apierrors.NewConflict(schema.GroupResource{Group: v1alpha1.GroupVersion.Group, Resource: "gpupools"}, "pool", errors.New("boom"))}

	if _, err := rec.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "ns", Name: "pool"}}); err == nil {
		t.Fatalf("expected API error")
	}
}
