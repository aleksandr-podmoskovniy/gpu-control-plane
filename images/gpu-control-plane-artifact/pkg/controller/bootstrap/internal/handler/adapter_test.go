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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap/internal/state"
)

type stubBootstrapHandler struct {
	name    string
	client  client.Client
	calls   int
	handled *v1alpha1.GPUNodeState
}

func (s *stubBootstrapHandler) Name() string { return s.name }

func (s *stubBootstrapHandler) HandleNode(_ context.Context, inventory *v1alpha1.GPUNodeState) (reconcile.Result, error) {
	s.calls++
	s.handled = inventory
	return reconcile.Result{}, nil
}

func (s *stubBootstrapHandler) SetClient(cl client.Client) { s.client = cl }

type handlerWithoutClientSetter struct{}

func (handlerWithoutClientSetter) Name() string { return "no-setter" }

func (handlerWithoutClientSetter) HandleNode(context.Context, *v1alpha1.GPUNodeState) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func TestWrapBootstrapHandlerDelegatesCalls(t *testing.T) {
	target := &stubBootstrapHandler{name: "stub"}
	adapter := WrapBootstrapHandler(target)

	if adapter.Name() != "stub" {
		t.Fatalf("unexpected adapter name: %s", adapter.Name())
	}

	inventory := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	s := state.New(nil, inventory)
	if _, err := adapter.Handle(context.Background(), s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if target.calls != 1 || target.handled != inventory {
		t.Fatalf("expected handler called once with inventory, got calls=%d handled=%v", target.calls, target.handled)
	}
}

func TestWrapBootstrapHandlerSetClientIsOptional(t *testing.T) {
	cl := clientfake.NewClientBuilder().Build()

	withSetter := &stubBootstrapHandler{name: "with"}
	withAdapter := WrapBootstrapHandler(withSetter).(*handlerAdapter)
	withAdapter.SetClient(cl)
	if withSetter.client != cl {
		t.Fatalf("expected client to be set on underlying handler")
	}

	withoutSetter := WrapBootstrapHandler(handlerWithoutClientSetter{}).(*handlerAdapter)
	withoutSetter.SetClient(cl) // should be a no-op
}
