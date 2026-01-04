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
)

const persistHandlerName = "persist"

// PersistHandler writes allocation results into the claim status.
type PersistHandler struct {
	writer *service.AllocationWriter
}

// NewPersistHandler constructs a persist handler.
func NewPersistHandler(writer *service.AllocationWriter) *PersistHandler {
	return &PersistHandler{writer: writer}
}

// Name returns the handler name.
func (h *PersistHandler) Name() string {
	return persistHandlerName
}

// Handle writes allocation result into ResourceClaim status.
func (h *PersistHandler) Handle(ctx context.Context, st *state.DRAState) (reconcile.Result, error) {
	if h.writer == nil || st.Allocation == nil {
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, h.writer.Write(ctx, st.Resource.Current(), st.Allocation)
}
