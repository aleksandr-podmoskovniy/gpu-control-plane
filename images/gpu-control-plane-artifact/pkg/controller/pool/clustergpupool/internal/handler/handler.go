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

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/pool/clustergpupool/internal/state"
)

type PoolHandler interface {
	Name() string
	HandlePool(ctx context.Context, pool *v1alpha1.GPUPool) (reconcile.Result, error)
}

type poolHandlerAdapter struct {
	handler PoolHandler
}

func WrapPoolHandler(handler PoolHandler) *poolHandlerAdapter {
	return &poolHandlerAdapter{handler: handler}
}

func (h *poolHandlerAdapter) Name() string {
	return h.handler.Name()
}

func (h *poolHandlerAdapter) Handle(ctx context.Context, s state.PoolState) (reconcile.Result, error) {
	return h.handler.HandlePool(ctx, s.Pool())
}
