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

package nvml

import (
	"context"
	"fmt"

	nvmlapi "github.com/NVIDIA/go-nvml/pkg/nvml"
)

// Checker verifies that GPUs are free of running processes using NVML.
type Checker struct {
	nvml nvmlapi.Interface
}

// Options configures the NVML checker.
type Options struct {
	NVML nvmlapi.Interface
}

// NewChecker constructs an NVML-backed GPU process checker.
func NewChecker(opts Options) *Checker {
	lib := opts.NVML
	if lib == nil {
		lib = nvmlapi.New(nvmlLibraryOptions()...)
	}
	return &Checker{nvml: lib}
}

// EnsureGPUFree fails if the GPU has active compute or graphics processes.
func (c *Checker) EnsureGPUFree(ctx context.Context, pciBusID string) error {
	if c == nil || c.nvml == nil {
		return fmt.Errorf("nvml checker is not configured")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	ret := c.nvml.Init()
	if ret != nvmlapi.SUCCESS && ret != nvmlapi.ERROR_ALREADY_INITIALIZED {
		return fmt.Errorf("nvml init failed: %s", c.nvml.ErrorString(ret))
	}
	defer func() {
		_ = c.nvml.Shutdown()
	}()

	normalized := normalizePCIBusID(pciBusID)
	device, ret := c.nvml.DeviceGetHandleByPciBusId(normalized)
	if ret != nvmlapi.SUCCESS {
		return fmt.Errorf("nvml device lookup failed for %q: %s", pciBusID, c.nvml.ErrorString(ret))
	}

	compute, ret := device.GetComputeRunningProcesses()
	if ret != nvmlapi.SUCCESS {
		return fmt.Errorf("nvml compute process query failed for %q: %s", pciBusID, c.nvml.ErrorString(ret))
	}
	if len(compute) > 0 {
		return fmt.Errorf("gpu %q has %d running compute process(es)", pciBusID, len(compute))
	}

	graphics, ret := device.GetGraphicsRunningProcesses()
	if ret != nvmlapi.SUCCESS {
		return fmt.Errorf("nvml graphics process query failed for %q: %s", pciBusID, c.nvml.ErrorString(ret))
	}
	if len(graphics) > 0 {
		return fmt.Errorf("gpu %q has %d running graphics process(es)", pciBusID, len(graphics))
	}

	return nil
}
