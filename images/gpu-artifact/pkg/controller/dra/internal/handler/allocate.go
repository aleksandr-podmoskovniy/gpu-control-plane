/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handler

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/dra/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/dra/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/reconciler"
)

const allocateHandlerName = "allocate"

// AllocateHandler computes allocation results for unallocated claims.
type AllocateHandler struct {
	allocator *service.Allocator
}

// NewAllocateHandler constructs an allocation handler.
func NewAllocateHandler(allocator *service.Allocator) *AllocateHandler {
	return &AllocateHandler{allocator: allocator}
}

// Name returns the handler name.
func (h *AllocateHandler) Name() string {
	return allocateHandlerName
}

// Handle computes allocation for the ResourceClaim when needed.
func (h *AllocateHandler) Handle(ctx context.Context, st *state.DRAState) (reconcile.Result, error) {
	if h.allocator == nil || st.Resource.IsEmpty() {
		return reconcile.Result{}, nil
	}

	claim := st.Resource.Current()
	if claim.Status.Allocation != nil {
		return reconcile.Result{}, reconciler.ErrStopHandlerChain
	}

	alloc, err := h.allocator.Allocate(ctx, claim)
	if err != nil {
		return reconcile.Result{}, err
	}
	if alloc == nil {
		return reconcile.Result{}, nil
	}

	st.Allocation = alloc
	return reconcile.Result{}, nil
}
