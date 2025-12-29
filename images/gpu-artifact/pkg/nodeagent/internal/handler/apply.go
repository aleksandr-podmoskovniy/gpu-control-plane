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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

const applyHandlerName = "Apply"

// ApplyHandler upserts PhysicalGPU objects and their status.
type ApplyHandler struct {
	store service.Store
}

// NewApplyHandler constructs an apply handler.
func NewApplyHandler(store service.Store) *ApplyHandler {
	return &ApplyHandler{store: store}
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
	obj, err := h.store.Get(ctx, name)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("get PhysicalGPU: %w", err)
	}

	if apierrors.IsNotFound(err) {
		obj = &gpuv1alpha1.PhysicalGPU{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: state.LabelsForDevice(nodeName, dev),
			},
		}
		if err := h.store.Create(ctx, obj); err != nil {
			return fmt.Errorf("create PhysicalGPU: %w", err)
		}
	} else {
		if err := h.ensureLabels(ctx, obj, nodeName, dev); err != nil {
			return fmt.Errorf("update PhysicalGPU labels: %w", err)
		}
	}

	desiredStatus := buildStatus(obj, dev, nodeName, nodeInfo)
	patchBase := obj.DeepCopy()
	obj.Status = desiredStatus
	if err := h.store.PatchStatus(ctx, obj, patchBase); err != nil {
		return fmt.Errorf("patch PhysicalGPU status: %w", err)
	}

	return nil
}
