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
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/builder"
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

func (h *ApplyHandler) applyDevice(ctx context.Context, name, nodeName string, dev state.Device, nodeInfo *gpuv1alpha1.NodeInfo) error {
	var log *slog.Logger
	logFor := func() *slog.Logger {
		if log == nil {
			log = deviceLog(ctx, nodeName, name, dev)
		}
		return log
	}
	obj, err := h.store.Get(ctx, name)
	if err != nil && !apierrors.IsNotFound(err) {
		logFor().Error("failed to get PhysicalGPU", logger.SlogErr(err))
		return fmt.Errorf("get PhysicalGPU: %w", err)
	}

	created := false
	if apierrors.IsNotFound(err) {
		construct := builder.NewPhysicalGPU(name)
		construct.SetLabels(state.LabelsForDevice(nodeName, dev))
		obj = construct.GetResource()
		if err := h.store.Create(ctx, obj); err != nil {
			logFor().Error("failed to create PhysicalGPU", logger.SlogErr(err))
			return fmt.Errorf("create PhysicalGPU: %w", err)
		}
		created = true
	} else {
		if err := h.ensureLabels(ctx, obj, nodeName, dev); err != nil {
			if h.recorder != nil {
				h.recordEvent(logFor(), obj, corev1.EventTypeWarning, reasonPhysicalGPULabelUpdateFailed, fmt.Sprintf("PhysicalGPU labels update failed: %v", err))
			}
			logFor().Error("failed to update PhysicalGPU labels", logger.SlogErr(err))
			return fmt.Errorf("update PhysicalGPU labels: %w", err)
		}
	}

	desiredStatus := buildStatus(obj, dev, nodeName, nodeInfo)
	patchBase := obj.DeepCopy()
	obj.Status = desiredStatus
	if err := h.store.PatchStatus(ctx, obj, patchBase); err != nil {
		if h.recorder != nil {
			h.recordEvent(logFor(), obj, corev1.EventTypeWarning, reasonPhysicalGPUStatusUpdateFailed, fmt.Sprintf("PhysicalGPU status update failed: %v", err))
		}
		logFor().Error("failed to update PhysicalGPU status", logger.SlogErr(err))
		return fmt.Errorf("patch PhysicalGPU status: %w", err)
	}

	if created {
		if h.recorder != nil {
			h.recordEvent(logFor(), obj, corev1.EventTypeNormal, reasonPhysicalGPUCreated, "PhysicalGPU created from PCI scan")
		}
	}

	return nil
}

func (h *ApplyHandler) recordEvent(log *slog.Logger, obj *gpuv1alpha1.PhysicalGPU, eventType, reason, message string) {
	if h.recorder == nil || obj == nil {
		return
	}
	if log == nil {
		h.recorder.Event(obj, eventType, reason, message)
		return
	}
	h.recorder.WithLogging(log).Event(obj, eventType, reason, message)
}

func deviceLog(ctx context.Context, nodeName, name string, dev state.Device) *slog.Logger {
	log := logger.FromContext(ctx).With("node", nodeName, "physicalgpu", name)
	if dev.Address != "" {
		log = log.With("pci", dev.Address)
	}
	if vendor := state.VendorLabel(dev); vendor != "" {
		log = log.With("vendor", vendor)
	}
	if device := state.DeviceLabel(dev.DeviceName); device != "" {
		log = log.With("device", device)
	}
	return log
}
