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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap/internal/state"
)

type BootstrapHandler interface {
	Name() string
	HandleNode(ctx context.Context, inventory *v1alpha1.GPUNodeState) (reconcile.Result, error)
}

type Handler interface {
	Name() string
	Handle(ctx context.Context, s state.NodeState) (reconcile.Result, error)
}

type handlerAdapter struct {
	handler BootstrapHandler
}

func WrapBootstrapHandler(handler BootstrapHandler) Handler {
	return &handlerAdapter{handler: handler}
}

func (h *handlerAdapter) Name() string {
	return h.handler.Name()
}

func (h *handlerAdapter) Handle(ctx context.Context, s state.NodeState) (reconcile.Result, error) {
	return h.handler.HandleNode(ctx, s.Inventory())
}

func (h *handlerAdapter) SetClient(cl client.Client) {
	if setter, ok := h.handler.(interface{ SetClient(client.Client) }); ok {
		setter.SetClient(cl)
	}
}
