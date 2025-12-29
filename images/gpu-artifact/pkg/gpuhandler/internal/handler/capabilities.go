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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
)

const capabilitiesHandlerName = "capabilities"

const (
	hardwareHealthyType       = "HardwareHealthy"
	reasonNVMLHealthy         = "NVMLHealthy"
	reasonNVMLUnavailable     = "NVMLUnavailable"
	reasonNVMLQueryFailed     = "NVMLQueryFailed"
	reasonDriverTypeNotNvidia = "DriverTypeNotNvidia"
	reasonMissingPCIAddress   = "MissingPCIAddress"
)

// CapabilitiesHandler enriches PhysicalGPU status using NVML.
type CapabilitiesHandler struct {
	reader  service.CapabilitiesReader
	store   *service.PhysicalGPUService
	tracker FailureTracker
}

// NewCapabilitiesHandler constructs a capabilities handler.
func NewCapabilitiesHandler(reader service.CapabilitiesReader, store *service.PhysicalGPUService, tracker FailureTracker) *CapabilitiesHandler {
	return &CapabilitiesHandler{
		reader:  reader,
		store:   store,
		tracker: tracker,
	}
}

// Name returns the handler name.
func (h *CapabilitiesHandler) Name() string {
	return capabilitiesHandlerName
}

// Handle enriches DriverReady GPUs with NVML capabilities and current state.
func (h *CapabilitiesHandler) Handle(ctx context.Context, st state.State) error {
	if h.reader == nil || h.store == nil || h.tracker == nil {
		return nil
	}

	ready := st.Ready()
	if len(ready) == 0 {
		return nil
	}

	var errs []error
	var nvidia []gpuv1alpha1.PhysicalGPU

	for _, pgpu := range ready {
		if !isDriverTypeNvidia(pgpu) {
			if err := h.markDriverTypeNotNvidia(ctx, pgpu); err != nil {
				errs = append(errs, err)
			}
			h.tracker.Clear(pgpu.Name)
			continue
		}

		if !h.tracker.ShouldAttempt(pgpu.Name) {
			continue
		}

		nvidia = append(nvidia, pgpu)
	}

	if len(nvidia) == 0 {
		return errors.Join(errs...)
	}

	session, err := h.reader.Open()
	if err != nil {
		errs = append(errs, h.applyFailure(ctx, nvidia, err))
		return errors.Join(errs...)
	}
	defer session.Close()

	for _, pgpu := range nvidia {
		if err := h.updateDevice(ctx, session, pgpu); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (h *CapabilitiesHandler) updateDevice(ctx context.Context, session service.CapabilitiesSession, pgpu gpuv1alpha1.PhysicalGPU) error {
	base := pgpu.DeepCopy()
	obj := pgpu.DeepCopy()

	pciAddress := ""
	if obj.Status.PCIInfo != nil {
		pciAddress = obj.Status.PCIInfo.Address
	}

	snapshot, err := session.ReadDevice(pciAddress)
	if err != nil {
		return h.applyDeviceFailure(ctx, obj, base, err)
	}

	obj.Status.Capabilities = snapshot.Capabilities
	obj.Status.CurrentState = mergeCurrentState(obj.Status.CurrentState, snapshot.CurrentState)
	h.setHardwareCondition(obj, metav1.ConditionTrue, reasonNVMLHealthy, "NVML is available")

	h.tracker.Clear(obj.Name)
	return h.store.PatchStatus(ctx, obj, base)
}

func (h *CapabilitiesHandler) markDriverTypeNotNvidia(ctx context.Context, pgpu gpuv1alpha1.PhysicalGPU) error {
	base := pgpu.DeepCopy()
	obj := pgpu.DeepCopy()

	if obj.Status.CurrentState != nil {
		obj.Status.CurrentState.Nvidia = nil
	}
	h.setHardwareCondition(obj, metav1.ConditionUnknown, reasonDriverTypeNotNvidia, "current driver is not Nvidia")
	return h.store.PatchStatus(ctx, obj, base)
}

func (h *CapabilitiesHandler) applyFailure(ctx context.Context, devices []gpuv1alpha1.PhysicalGPU, err error) error {
	var errs []error
	for _, pgpu := range devices {
		base := pgpu.DeepCopy()
		obj := pgpu.DeepCopy()
		if applyErr := h.applyDeviceFailure(ctx, obj, base, err); applyErr != nil {
			errs = append(errs, applyErr)
		}
	}
	return errors.Join(errs...)
}

func (h *CapabilitiesHandler) applyDeviceFailure(ctx context.Context, obj, base *gpuv1alpha1.PhysicalGPU, err error) error {
	if !h.tracker.RecordFailure(obj.Name) {
		return nil
	}

	reason := failureReason(err)
	h.setHardwareCondition(obj, metav1.ConditionUnknown, reason, err.Error())
	return h.store.PatchStatus(ctx, obj, base)
}

func (h *CapabilitiesHandler) setHardwareCondition(obj *gpuv1alpha1.PhysicalGPU, status metav1.ConditionStatus, reason, message string) {
	cond := metav1.Condition{
		Type:               hardwareHealthyType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: obj.Generation,
	}
	meta.SetStatusCondition(&obj.Status.Conditions, cond)
}

func failureReason(err error) string {
	switch {
	case errors.Is(err, service.ErrMissingPCIAddress):
		return reasonMissingPCIAddress
	case errors.Is(err, service.ErrNVMLUnavailable):
		return reasonNVMLUnavailable
	case errors.Is(err, service.ErrNVMLQueryFailed):
		return reasonNVMLQueryFailed
	default:
		return reasonNVMLQueryFailed
	}
}

func mergeCurrentState(existing, snapshot *gpuv1alpha1.GPUCurrentState) *gpuv1alpha1.GPUCurrentState {
	driverType := gpuv1alpha1.DriverType("")
	if existing != nil {
		driverType = existing.DriverType
	}
	if snapshot == nil {
		snapshot = &gpuv1alpha1.GPUCurrentState{}
	}
	snapshot.DriverType = driverType
	return snapshot
}

func isDriverTypeNvidia(pgpu gpuv1alpha1.PhysicalGPU) bool {
	if pgpu.Status.CurrentState == nil {
		return false
	}
	return pgpu.Status.CurrentState.DriverType == gpuv1alpha1.DriverTypeNvidia
}
