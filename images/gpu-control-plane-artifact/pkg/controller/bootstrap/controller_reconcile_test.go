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

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	bshandler "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap/internal/handler"
	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	bootmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics/bootstrap"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/validation"
)

func TestReconcileAggregatesResults(t *testing.T) {
	scheme := newScheme(t)
	node := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()

	handlerA := &stubBootstrapHandler{name: "a", result: reconcile.Result{Requeue: true}}
	handlerB := &stubBootstrapHandler{name: "b", result: reconcile.Result{RequeueAfter: time.Second}}

	rec := New(testr.New(t), config.ControllerConfig{}, nil, []Handler{bshandler.WrapBootstrapHandler(handlerA), bshandler.WrapBootstrapHandler(handlerB)})
	rec.client = client

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("unexpected aggregate result: %+v", res)
	}
	if handlerA.calls != 1 || handlerB.calls != 1 {
		t.Fatalf("expected handlers invoked once, got %d/%d", handlerA.calls, handlerB.calls)
	}
}

func TestReconcileHandlerError(t *testing.T) {
	scheme := newScheme(t)
	node := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()

	handlerName := "boom-" + t.Name()
	handler := &stubBootstrapHandler{name: handlerName, err: errors.New("handler fail")}
	rec := New(testr.New(t), config.ControllerConfig{}, nil, []Handler{bshandler.WrapBootstrapHandler(handler)})
	rec.client = client

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}}); err == nil {
		t.Fatal("expected handler error")
	}
	if handler.calls != 1 {
		t.Fatalf("expected handler called once, got %d", handler.calls)
	}
	if v, ok := counterValue(t, bootmetrics.BootstrapHandlerErrorsTotal, map[string]string{"handler": handlerName}); !ok || v != 1 {
		t.Fatalf("expected handler error metric incremented, got %f", v)
	}
}

func TestReconcileSkipsWhenModuleDisabled(t *testing.T) {
	scheme := newScheme(t)
	inventory := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(inventory).Build()
	store := moduleconfig.NewModuleConfigStore(moduleconfig.State{Enabled: false, Settings: moduleconfig.DefaultState().Settings})

	rec := New(testr.New(t), config.ControllerConfig{}, store, nil)
	rec.client = client
	rec.validator = &stubValidator{}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := rec.validator.(*stubValidator)
	if val.statusCalls != 0 {
		t.Fatalf("expected validator not to be called, got status=%d", val.statusCalls)
	}
}

func TestReconcileGetError(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingClient{err: errors.New("get fail")}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}}); err == nil {
		t.Fatal("expected get error")
	}
}

func TestReconcileNotFound(t *testing.T) {
	scheme := newScheme(t)
	client := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = client

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing"}}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestReconcileNoHandlers(t *testing.T) {
	scheme := newScheme(t)
	node := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = client

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected empty result, got %+v", res)
	}
}

func TestReconcilePersistsStatusChanges(t *testing.T) {
	scheme := newScheme(t)
	node := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node).
		WithStatusSubresource(&v1alpha1.GPUNodeState{}).
		Build()

	rec := New(testr.New(t), config.ControllerConfig{}, nil, []Handler{bshandler.WrapBootstrapHandler(statusChangingHandler{})})
	rec.client = client

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &v1alpha1.GPUNodeState{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "node"}, updated); err != nil {
		t.Fatalf("get inventory: %v", err)
	}
	if len(updated.Status.Conditions) != 1 || updated.Status.Conditions[0].Type != "Ready" {
		t.Fatalf("expected condition persisted, got %+v", updated.Status.Conditions)
	}
}

func TestReconcileLogsValidatorStatusErrorsAndUsesSpecNodeName(t *testing.T) {
	scheme := newScheme(t)
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-validator-error"},
		Spec:       v1alpha1.GPUNodeStateSpec{NodeName: "node-from-spec"},
	}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(inventory).Build()

	validator := &capturingValidator{err: errors.New("validator status failed")}
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = cl
	rec.validator = validator

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: inventory.Name}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if validator.statusCalls != 1 || validator.nodeName != "node-from-spec" {
		t.Fatalf("expected validator called with node-from-spec once, got calls=%d node=%q", validator.statusCalls, validator.nodeName)
	}
}

func TestReconcilePassesValidatorStatusToHandlers(t *testing.T) {
	scheme := newScheme(t)
	inventory := &v1alpha1.GPUNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "node-validator-status"},
	}
	cl := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(inventory).Build()

	validator := &capturingValidator{result: validation.Result{DriverReady: true}}
	handler := &statusReadingHandler{}

	rec := New(testr.New(t), config.ControllerConfig{}, nil, []Handler{bshandler.WrapBootstrapHandler(handler)})
	rec.client = cl
	rec.validator = validator

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: inventory.Name}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handler.present || !handler.status.DriverReady {
		t.Fatalf("expected validator status propagated to handler, got present=%t status=%+v", handler.present, handler.status)
	}
}

func TestReconcileWrapsAPIError(t *testing.T) {
	rec := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	rec.client = &failingClient{err: apierrors.NewConflict(schema.GroupResource{Group: v1alpha1.GroupVersion.Group, Resource: "gpunodestates"}, "node", errors.New("boom"))}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}}); err == nil {
		t.Fatal("expected API error")
	}
}
