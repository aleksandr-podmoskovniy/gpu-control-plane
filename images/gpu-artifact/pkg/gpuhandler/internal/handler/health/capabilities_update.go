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

package health

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/capabilities"
)

func (h *CapabilitiesHandler) updateDevice(ctx context.Context, session capabilities.CapabilitiesSession, pgpu gpuv1alpha1.PhysicalGPU) (*gpuv1alpha1.PhysicalGPU, error) {
	base := pgpu.DeepCopy()
	obj := pgpu.DeepCopy()

	pciAddress := ""
	if obj.Status.PCIInfo != nil {
		pciAddress = obj.Status.PCIInfo.Address
	}

	snapshot, err := session.ReadDevice(pciAddress)
	if err != nil {
		return nil, h.applyDeviceFailure(ctx, obj, base, err)
	}

	obj.Status.Capabilities = snapshot.Capabilities
	obj.Status.CurrentState = mergeCurrentState(obj.Status.CurrentState, snapshot.CurrentState)
	setHardwareCondition(obj, metav1.ConditionTrue, reasonNVMLHealthy, "NVML is available")

	h.tracker.Clear(obj.Name)
	if err := h.store.PatchStatus(ctx, obj, base); err != nil {
		return nil, err
	}
	return obj, nil
}

func (h *CapabilitiesHandler) markDriverTypeNotNvidia(ctx context.Context, pgpu gpuv1alpha1.PhysicalGPU) error {
	base := pgpu.DeepCopy()
	obj := pgpu.DeepCopy()

	if obj.Status.CurrentState != nil {
		obj.Status.CurrentState.Nvidia = nil
	}
	setHardwareCondition(obj, metav1.ConditionUnknown, reasonDriverTypeNotNvidia, "current driver is not Nvidia")
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
	setHardwareCondition(obj, metav1.ConditionUnknown, reason, err.Error())
	h.recordHardwareEvent(ctx, obj, base, reason, err.Error())
	return h.store.PatchStatus(ctx, obj, base)
}
