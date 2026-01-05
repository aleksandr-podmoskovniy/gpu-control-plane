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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

const markNotReadyHandlerName = "mark-not-ready"
const reasonToolkitNotReady = "ToolkitNotReady"

// MarkNotReadyHandler sets HardwareHealthy=Unknown for non-ready GPUs.
type MarkNotReadyHandler struct {
	store    *service.PhysicalGPUService
	tracker  FailureTracker
	recorder eventrecord.EventRecorderLogger
}

// NewMarkNotReadyHandler constructs a handler for non-ready GPUs.
func NewMarkNotReadyHandler(store *service.PhysicalGPUService, tracker FailureTracker, recorder eventrecord.EventRecorderLogger) *MarkNotReadyHandler {
	return &MarkNotReadyHandler{
		store:    store,
		tracker:  tracker,
		recorder: recorder,
	}
}

// Name returns the handler name.
func (h *MarkNotReadyHandler) Name() string {
	return markNotReadyHandlerName
}

// Handle marks non-ready GPUs as HardwareHealthy Unknown.
func (h *MarkNotReadyHandler) Handle(ctx context.Context, st state.State) error {
	if h.store == nil {
		return nil
	}

	var errs []error
	for _, pgpu := range st.All() {
		cond := meta.FindStatusCondition(pgpu.Status.Conditions, driverReadyType)
		if cond != nil && cond.Status == metav1.ConditionTrue {
			continue
		}

		if h.tracker != nil {
			h.tracker.Clear(pgpu.Name)
		}

		base := pgpu.DeepCopy()
		obj := pgpu.DeepCopy()

		setHardwareConditionUnknown(obj, reasonDriverNotReady, "driver is not ready")

		h.recordToolkitNotReadyEvent(ctx, obj, base)
		if err := h.store.PatchStatus(ctx, obj, base); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func setHardwareConditionUnknown(obj *gpuv1alpha1.PhysicalGPU, reason, message string) {
	cond := metav1.Condition{
		Type:               hardwareHealthyType,
		Status:             metav1.ConditionUnknown,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: obj.Generation,
	}
	meta.SetStatusCondition(&obj.Status.Conditions, cond)
}

func (h *MarkNotReadyHandler) recordToolkitNotReadyEvent(ctx context.Context, obj, base *gpuv1alpha1.PhysicalGPU) {
	if h.recorder == nil || obj == nil || base == nil {
		return
	}

	if !hardwareConditionChanged(base, obj) {
		return
	}

	log := logger.FromContext(ctx)
	if obj.Status.NodeInfo != nil && obj.Status.NodeInfo.NodeName != "" {
		log = log.With("node", obj.Status.NodeInfo.NodeName)
	}

	h.recorder.WithLogging(log).Event(
		obj,
		corev1.EventTypeWarning,
		reasonToolkitNotReady,
		"driver is not ready",
	)
}
