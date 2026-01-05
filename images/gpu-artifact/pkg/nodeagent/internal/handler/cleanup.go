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
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

const cleanupHandlerName = "Cleanup"

// CleanupHandler deletes stale PhysicalGPU objects.
type CleanupHandler struct {
	store    service.Store
	recorder eventrecord.EventRecorderLogger
}

// NewCleanupHandler constructs a cleanup handler.
func NewCleanupHandler(store service.Store, recorder eventrecord.EventRecorderLogger) *CleanupHandler {
	return &CleanupHandler{store: store, recorder: recorder}
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
		var log *slog.Logger
		logFor := func() *slog.Logger {
			if log == nil {
				log = physicalGPULog(ctx, st.NodeName(), obj)
			}
			return log
		}
		if err := h.store.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			if h.recorder != nil {
				h.recordEvent(logFor(), obj, corev1.EventTypeWarning, reasonPhysicalGPUDeleteFailed, fmt.Sprintf("PhysicalGPU delete failed: %v", err))
			}
			logFor().Error("failed to delete PhysicalGPU", logger.SlogErr(err))
			errs = append(errs, fmt.Errorf("delete %s: %w", obj.Name, err))
			continue
		}
		if h.recorder != nil {
			h.recordEvent(logFor(), obj, corev1.EventTypeNormal, reasonPhysicalGPUDeleted, "PhysicalGPU removed from PCI scan")
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

func (h *CleanupHandler) recordEvent(log *slog.Logger, obj *gpuv1alpha1.PhysicalGPU, eventType, reason, message string) {
	if h.recorder == nil || obj == nil {
		return
	}
	if log == nil {
		h.recorder.Event(obj, eventType, reason, message)
		return
	}
	h.recorder.WithLogging(log).Event(obj, eventType, reason, message)
}

func physicalGPULog(ctx context.Context, nodeName string, obj *gpuv1alpha1.PhysicalGPU) *slog.Logger {
	log := logger.FromContext(ctx).With("node", nodeName, "physicalgpu", obj.Name)
	if obj.Status.PCIInfo != nil && obj.Status.PCIInfo.Address != "" {
		log = log.With("pci", obj.Status.PCIInfo.Address)
	}
	if obj.Labels != nil {
		if vendor := obj.Labels[state.LabelVendor]; vendor != "" {
			log = log.With("vendor", vendor)
		}
		if device := obj.Labels[state.LabelDevice]; device != "" {
			log = log.With("device", device)
		}
	}
	return log
}
