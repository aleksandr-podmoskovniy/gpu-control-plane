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
	"errors"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/capabilities"
)

func failureReason(err error) string {
	switch {
	case errors.Is(err, capabilities.ErrMissingPCIAddress):
		return reasonMissingPCIAddress
	case errors.Is(err, capabilities.ErrNVMLUnavailable):
		return reasonNVMLUnavailable
	case errors.Is(err, capabilities.ErrNVMLQueryFailed):
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
