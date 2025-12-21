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

package internal

import (
	"context"
	"errors"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/clustergpupool/internal/state"
)

type stubPoolHandler struct {
	name    string
	calls   int
	handled *v1alpha1.GPUPool
	result  contracts.Result
	err     error
}

func (s *stubPoolHandler) Name() string { return s.name }

func (s *stubPoolHandler) HandlePool(_ context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	s.calls++
	s.handled = pool
	return s.result, s.err
}

func TestWrapPoolHandlerDelegatesCalls(t *testing.T) {
	expectedErr := errors.New("boom")
	target := &stubPoolHandler{name: "stub", result: reconcile.Result{RequeueAfter: 1}, err: expectedErr}
	adapter := WrapPoolHandler(target)

	if adapter.Name() != "stub" {
		t.Fatalf("unexpected name: %s", adapter.Name())
	}

	pool := &v1alpha1.GPUPool{}
	st := state.New(nil, pool)
	_, err := adapter.Handle(context.Background(), st)

	if err != expectedErr {
		t.Fatalf("expected error to be propagated")
	}
	if target.calls != 1 || target.handled != pool {
		t.Fatalf("expected underlying handler invoked once with pool")
	}
}
