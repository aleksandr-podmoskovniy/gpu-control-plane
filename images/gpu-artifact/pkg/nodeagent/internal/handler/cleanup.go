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
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

const cleanupHandlerName = "Cleanup"

// CleanupHandler deletes stale PhysicalGPU objects.
type CleanupHandler struct {
	store service.Store
}

// NewCleanupHandler constructs a cleanup handler.
func NewCleanupHandler(store service.Store) *CleanupHandler {
	return &CleanupHandler{store: store}
}

// Name returns the handler name.
func (h *CleanupHandler) Name() string {
	return cleanupHandlerName
}

// Handle removes objects not present in the expected set.
func (h *CleanupHandler) Handle(ctx context.Context, st state.State) error {
	items, err := h.store.ListByNode(ctx, st.NodeName())
	if err != nil {
		return fmt.Errorf("list PhysicalGPU: %w", err)
	}

	expected := st.Expected()
	if expected == nil {
		expected = map[string]state.Device{}
	}

	var errs []error
	for i := range items {
		obj := &items[i]
		if _, ok := expected[obj.Name]; ok {
			continue
		}
		if err := h.store.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete %s: %w", obj.Name, err))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
