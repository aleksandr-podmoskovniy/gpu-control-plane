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

package apply

import (
	"context"
	"errors"
	"fmt"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

const applyHandlerName = "Apply"

// ApplyHandler upserts PhysicalGPU objects and their status.
type ApplyHandler struct {
	store    service.Store
	recorder eventrecord.EventRecorderLogger
}

// NewApplyHandler constructs an apply handler.
func NewApplyHandler(store service.Store, recorder eventrecord.EventRecorderLogger) *ApplyHandler {
	return &ApplyHandler{store: store, recorder: recorder}
}

// Name returns the handler name.
func (h *ApplyHandler) Name() string {
	return applyHandlerName
}

// Handle reconciles PhysicalGPU objects for all detected devices.
func (h *ApplyHandler) Handle(ctx context.Context, st state.State) error {
	var errs []error
	for _, dev := range st.Devices() {
		name := state.PhysicalGPUName(st.NodeName(), dev)
		if err := h.applyDevice(ctx, name, st.NodeName(), dev, st.NodeInfo()); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
