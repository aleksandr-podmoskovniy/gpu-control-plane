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

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
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
	reasonDriverNotReady      = "DriverNotReady"
	reasonMissingPCIAddress   = "MissingPCIAddress"
)

// CapabilitiesHandler enriches PhysicalGPU status using NVML.
type CapabilitiesHandler struct {
	reader   service.CapabilitiesReader
	store    *service.PhysicalGPUService
	tracker  FailureTracker
	recorder eventrecord.EventRecorderLogger
}

// NewCapabilitiesHandler constructs a capabilities handler.
func NewCapabilitiesHandler(reader service.CapabilitiesReader, store *service.PhysicalGPUService, tracker FailureTracker, recorder eventrecord.EventRecorderLogger) *CapabilitiesHandler {
	return &CapabilitiesHandler{
		reader:   reader,
		store:    store,
		tracker:  tracker,
		recorder: recorder,
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
	updated := make([]gpuv1alpha1.PhysicalGPU, 0, len(ready))

	for _, pgpu := range ready {
		if !isDriverTypeNvidia(pgpu) {
			if err := h.markDriverTypeNotNvidia(ctx, pgpu); err != nil {
				errs = append(errs, err)
			}
			h.tracker.Clear(pgpu.Name)
			updated = append(updated, pgpu)
			continue
		}

		if !h.tracker.ShouldAttempt(pgpu.Name) {
			updated = append(updated, pgpu)
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
		updatedPGPU, err := h.updateDevice(ctx, session, pgpu)
		if err != nil {
			errs = append(errs, err)
			updated = append(updated, pgpu)
			continue
		}
		if updatedPGPU != nil {
			updated = append(updated, *updatedPGPU)
		} else {
			updated = append(updated, pgpu)
		}
	}

	if len(updated) > 0 {
		st.SetReady(updated)
	}

	return errors.Join(errs...)
}
