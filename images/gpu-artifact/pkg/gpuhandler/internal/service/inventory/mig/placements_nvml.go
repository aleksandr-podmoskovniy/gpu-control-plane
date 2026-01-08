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

package mig

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	invtypes "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory/types"
	nvmlsvc "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/nvml"
)

// NVMLMigPlacementReader reads MIG placements via NVML.
type NVMLMigPlacementReader struct {
	nvml nvmlsvc.NVML
}

// NewNVMLMigPlacementReader constructs a MIG placement reader.
func NewNVMLMigPlacementReader(nvmlService nvmlsvc.NVML) *NVMLMigPlacementReader {
	return &NVMLMigPlacementReader{nvml: nvmlService}
}

// Open initializes NVML and returns a placement session.
func (r *NVMLMigPlacementReader) Open() (invtypes.MigPlacementSession, error) {
	if r.nvml == nil {
		return nil, newReadError(ErrNVMLUnavailable, "NVML is not configured")
	}

	ret := r.nvml.Init()
	if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return nil, newReadError(ErrNVMLUnavailable, "NVML init failed: %s", r.nvml.ErrorString(ret))
	}

	return &nvmlMigPlacementSession{nvml: r.nvml}, nil
}
