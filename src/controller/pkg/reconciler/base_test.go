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

package reconciler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

type stubHandler struct {
	name           string
	result         contracts.Result
	err            error
	stop           bool
	finalizeErr    error
	calls          int
	finalizerCalls int
}

func (h *stubHandler) Name() string {
	if h.name != "" {
		return h.name
	}
	return "stub"
}

func (h *stubHandler) Finalize(context.Context) error {
	h.finalizerCalls++
	return h.finalizeErr
}

func newContext(t *testing.T) context.Context {
	t.Helper()
	return logr.NewContext(context.Background(), testr.New(t))
}

func TestReconcileRequiresResourceUpdater(t *testing.T) {
	rec := NewBase([]*stubHandler{{}})
	rec.SetHandlerExecutor(func(context.Context, *stubHandler) (contracts.Result, error) {
		return contracts.Result{}, nil
	})

	if _, err := rec.Reconcile(newContext(t)); err == nil || err.Error() != "resource updater is not configured" {
		t.Fatalf("expected missing updater error, got %v", err)
	}
}

func TestReconcileRequiresHandlerExecutor(t *testing.T) {
	rec := NewBase([]*stubHandler{{}})
	rec.SetResourceUpdater(func(context.Context) error { return nil })

	if _, err := rec.Reconcile(newContext(t)); err == nil || err.Error() != "handler executor is not configured" {
		t.Fatalf("expected missing executor error, got %v", err)
	}
}

func TestReconcileSuccessPath(t *testing.T) {
	handlers := []*stubHandler{
		{name: "first", result: contracts.Result{RequeueAfter: time.Second}},
		{name: "second", result: contracts.Result{Requeue: true}},
	}
	rec := NewBase(handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h *stubHandler) (contracts.Result, error) {
		h.calls++
		return h.result, nil
	})
	updateCalls := 0
	rec.SetResourceUpdater(func(context.Context) error {
		updateCalls++
		return nil
	})

	result, err := rec.Reconcile(newContext(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Requeue || result.RequeueAfter != time.Second {
		t.Fatalf("unexpected result: %+v", result)
	}
	if handlers[0].calls != 1 || handlers[1].calls != 1 {
		t.Fatalf("expected both handlers called once, got %d/%d", handlers[0].calls, handlers[1].calls)
	}
	if updateCalls != 1 {
		t.Fatalf("expected updater invoked once, got %d", updateCalls)
	}
}

func TestReconcileStopChainSkipsRemainingHandlers(t *testing.T) {
	handlers := []*stubHandler{
		{name: "stop", result: contracts.Result{RequeueAfter: time.Millisecond}, stop: true},
		{name: "skip"},
	}
	rec := NewBase(handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h *stubHandler) (contracts.Result, error) {
		h.calls++
		if h.stop {
			return h.result, ErrStopChain
		}
		return h.result, nil
	})
	rec.SetResourceUpdater(func(context.Context) error { return nil })

	result, err := rec.Reconcile(newContext(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handlers[0].calls != 1 {
		t.Fatalf("expected first handler called once, got %d", handlers[0].calls)
	}
	if handlers[1].calls != 0 {
		t.Fatalf("expected second handler skipped, got %d", handlers[1].calls)
	}
	if result.RequeueAfter != time.Millisecond {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestReconcileHandlerConflictRequestsRequeue(t *testing.T) {
	conflict := apierrors.NewConflict(
		schema.GroupResource{Group: "gpu.deckhouse.io", Resource: "gpudevices"},
		"test",
		fmt.Errorf("conflict"),
	)
	handler := &stubHandler{name: "conflict", err: conflict}
	rec := NewBase([]*stubHandler{handler})
	rec.SetHandlerExecutor(func(ctx context.Context, h *stubHandler) (contracts.Result, error) {
		h.calls++
		return h.result, h.err
	})
	rec.SetResourceUpdater(func(context.Context) error { return nil })

	result, err := rec.Reconcile(newContext(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 100*time.Microsecond {
		t.Fatalf("expected conflict to set 100µs requeue, got %s", result.RequeueAfter)
	}
}

func TestReconcileUpdateConflictRequestsRequeue(t *testing.T) {
	conflict := apierrors.NewConflict(
		schema.GroupResource{Group: "gpu.deckhouse.io", Resource: "gpudevices"},
		"update",
		fmt.Errorf("conflict"),
	)
	rec := NewBase([]*stubHandler{{name: "one"}})
	rec.SetHandlerExecutor(func(ctx context.Context, h *stubHandler) (contracts.Result, error) {
		h.calls++
		return contracts.Result{}, nil
	})
	rec.SetResourceUpdater(func(context.Context) error { return conflict })

	result, err := rec.Reconcile(newContext(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 100*time.Microsecond {
		t.Fatalf("expected conflict to set 100µs requeue, got %s", result.RequeueAfter)
	}
}

func TestReconcileHandlerErrorStopsFurtherHandlers(t *testing.T) {
	errA := errors.New("first failed")
	errB := errors.New("second failed")

	handlers := []*stubHandler{{name: "a", err: errA}, {name: "b", err: errB}}
	rec := NewBase(handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h *stubHandler) (contracts.Result, error) {
		h.calls++
		return contracts.Result{}, h.err
	})
	rec.SetResourceUpdater(func(context.Context) error { return nil })

	_, err := rec.Reconcile(newContext(t))
	if !errors.Is(err, errA) {
		t.Fatalf("expected first error, got %v", err)
	}
	if handlers[1].calls != 0 {
		t.Fatalf("expected second handler not executed, got %d calls", handlers[1].calls)
	}
}

func TestReconcileAggregatesUpdateAndHandlerErrors(t *testing.T) {
	handlerErr := errors.New("handler failed")
	updateErr := errors.New("update failed")

	handlers := []*stubHandler{{name: "a", err: handlerErr}}
	rec := NewBase(handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h *stubHandler) (contracts.Result, error) {
		h.calls++
		return contracts.Result{}, h.err
	})
	rec.SetResourceUpdater(func(context.Context) error { return updateErr })

	_, err := rec.Reconcile(newContext(t))
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	if !errors.Is(err, handlerErr) || !errors.Is(err, updateErr) {
		t.Fatalf("expected joined error, got %v", err)
	}
}

func TestReconcileFinalizerErrorPropagates(t *testing.T) {
	finalizeErr := errors.New("finalize failed")
	handler := &stubHandler{name: "finalizer", finalizeErr: finalizeErr}

	rec := NewBase([]*stubHandler{handler})
	rec.SetHandlerExecutor(func(ctx context.Context, h *stubHandler) (contracts.Result, error) {
		return contracts.Result{}, nil
	})
	rec.SetResourceUpdater(func(context.Context) error { return nil })

	_, err := rec.Reconcile(newContext(t))
	if !errors.Is(err, finalizeErr) {
		t.Fatalf("expected finalizer error, got %v", err)
	}
	if handler.finalizerCalls != 1 {
		t.Fatalf("expected Finalize invoked once, got %d", handler.finalizerCalls)
	}
}

func TestReconcileSkipsNonFinalizerHandlers(t *testing.T) {
	type plain struct {
		calls int
	}

	rec := NewBase([]*plain{{}})
	rec.SetHandlerExecutor(func(ctx context.Context, h *plain) (contracts.Result, error) {
		h.calls++
		return contracts.Result{}, nil
	})
	rec.SetResourceUpdater(func(context.Context) error { return nil })

	if _, err := rec.Reconcile(newContext(t)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMergeResultsChoosesShortestDelay(t *testing.T) {
	res := MergeResults(
		contracts.Result{RequeueAfter: 5 * time.Second},
		contracts.Result{RequeueAfter: 2 * time.Second},
		contracts.Result{Requeue: true},
	)

	if !res.Requeue || res.RequeueAfter != 2*time.Second {
		t.Fatalf("unexpected merged result: %+v", res)
	}
}
