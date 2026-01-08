//go:build linux && cgo && nvml
// +build linux,cgo,nvml

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

package nvcdi

import (
	"context"
	"errors"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// Write generates CDI specs for a claim and returns CDI device ids per device.
func (w *Writer) Write(_ context.Context, req domain.PrepareRequest) (map[string][]string, error) {
	if w == nil {
		return nil, errors.New("CDI writer is nil")
	}
	if req.ClaimUID == "" {
		return nil, errors.New("claim UID is required")
	}
	if len(req.Devices) == 0 {
		return nil, errors.New("no devices to prepare")
	}
	if req.VFIO {
		return nil, errors.New("vfio prepare is not implemented")
	}

	if err := w.initNVML(); err != nil {
		return nil, err
	}
	defer w.nvml.Shutdown()

	commonEdits, err := w.commonEdits(w.nvcdiClaim)
	if err != nil {
		return nil, err
	}

	deviceSpecs, deviceIDs, err := w.buildDeviceSpecs(req)
	if err != nil {
		return nil, err
	}
	if len(deviceSpecs) == 0 {
		return nil, errors.New("no CDI device specs generated")
	}

	spec, err := buildSpec(w.vendor, w.claimClass, deviceSpecs, commonEdits.ContainerEdits)
	if err != nil {
		return nil, err
	}

	specName := cdiapi.GenerateTransientSpecName(w.vendor, w.claimClass, req.ClaimUID)
	if err := w.writeSpec(spec, specName); err != nil {
		return nil, err
	}

	return deviceIDs, nil
}

// Delete removes CDI specs for a claim.
func (w *Writer) Delete(_ context.Context, claimUID string) error {
	if w == nil {
		return errors.New("CDI writer is nil")
	}
	if claimUID == "" {
		return errors.New("claim UID is required")
	}
	specName := cdiapi.GenerateTransientSpecName(w.vendor, w.claimClass, claimUID)
	return w.cache.RemoveSpec(specName)
}

func (w *Writer) initNVML() error {
	ret := w.nvml.Init()
	if ret == nvml.SUCCESS || ret == nvml.ERROR_ALREADY_INITIALIZED {
		return nil
	}
	return fmt.Errorf("NVML init failed: %s", nvml.ErrorString(ret))
}
