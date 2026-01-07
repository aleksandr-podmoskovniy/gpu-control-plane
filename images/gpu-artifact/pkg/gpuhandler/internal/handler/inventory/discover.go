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

package inventory

import (
	"context"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
)

const discoverHandlerName = "discover"

// DiscoverHandler loads PhysicalGPU objects for the node.
type DiscoverHandler struct {
	service *service.PhysicalGPUService
}

// NewDiscoverHandler constructs a discover handler.
func NewDiscoverHandler(service *service.PhysicalGPUService) *DiscoverHandler {
	return &DiscoverHandler{service: service}
}

// Name returns the handler name.
func (h *DiscoverHandler) Name() string {
	return discoverHandlerName
}

// Handle loads PhysicalGPU objects and stores them in state.
func (h *DiscoverHandler) Handle(ctx context.Context, st state.State) error {
	if h.service == nil {
		return nil
	}
	devices, err := h.service.ListByNode(ctx, st.NodeName())
	if err != nil {
		return err
	}
	st.SetAll(devices)
	return nil
}
