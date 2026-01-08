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

package step

import (
	"context"
	"errors"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	prepdevice "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/internal/device"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// GPUFreeCheckStep ensures GPU devices are idle before VFIO.
type GPUFreeCheckStep struct {
	checker ports.GPUProcessChecker
}

// NewGPUFreeCheckStep constructs a GPU free check step.
func NewGPUFreeCheckStep(checker ports.GPUProcessChecker) GPUFreeCheckStep {
	return GPUFreeCheckStep{checker: checker}
}

func (s GPUFreeCheckStep) Take(ctx context.Context, st *state.PrepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("prepare state is nil")
	}
	if !st.Request.VFIO {
		return nil, nil
	}
	if s.checker == nil {
		return nil, errors.New("gpu process checker is not configured")
	}

	seen := make(map[string]struct{})
	for _, dev := range st.Request.Devices {
		deviceType := prepdevice.AttrString(dev.Attributes, allocatable.AttrDeviceType)
		if !prepdevice.IsPhysicalDevice(deviceType) {
			continue
		}
		pci := prepdevice.AttrString(dev.Attributes, allocatable.AttrPCIAddress)
		if pci == "" {
			return nil, fmt.Errorf("pci address is missing for device %q", dev.Device)
		}
		if _, ok := seen[pci]; ok {
			continue
		}
		if err := s.checker.EnsureGPUFree(ctx, pci); err != nil {
			return nil, err
		}
		seen[pci] = struct{}{}
	}
	return nil, nil
}
