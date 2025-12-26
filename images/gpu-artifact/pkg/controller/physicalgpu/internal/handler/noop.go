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

	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/physicalgpu/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
)

// NoopHandler is a placeholder handler for the PhysicalGPU controller.
type NoopHandler struct {
	recorder eventrecord.EventRecorderLogger
}

// NewNoopHandler creates a new NoopHandler.
func NewNoopHandler(recorder eventrecord.EventRecorderLogger) *NoopHandler {
	return &NoopHandler{recorder: recorder}
}

// Name returns the handler name.
func (h *NoopHandler) Name() string {
	return "Noop"
}

// Handle performs no action and returns an empty result.
func (h *NoopHandler) Handle(_ context.Context, _ state.PhysicalGPUState) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
