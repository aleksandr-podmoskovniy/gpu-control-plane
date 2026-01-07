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
	"io"
	"os"
	"strings"

	nvdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvcdiapi "github.com/NVIDIA/nvidia-container-toolkit/pkg/nvcdi"
	"github.com/sirupsen/logrus"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// Writer generates CDI specs using NVIDIA nvcdi.
type Writer struct {
	nvml             nvml.Interface
	nvcdi            nvcdiapi.Interface
	cache            *cdiapi.Cache
	driverRoot       string
	targetDriverRoot string
	vendor           string
	class            string
}

// New creates a CDI writer backed by nvcdi.
func New(opts Options) (*Writer, error) {
	vendor := strings.TrimSpace(opts.Vendor)
	if vendor == "" && opts.DriverName != "" {
		vendor = fmt.Sprintf("k8s.%s", opts.DriverName)
	}
	if vendor == "" {
		return nil, errors.New("vendor is required")
	}
	class := strings.TrimSpace(opts.Class)
	if class == "" {
		class = defaultClass
	}
	cdiRoot := strings.TrimSpace(opts.CDIRoot)
	if cdiRoot == "" {
		cdiRoot = defaultCDIRoot
	}
	driverRoot := strings.TrimSpace(opts.DriverRoot)
	if driverRoot == "" {
		driverRoot = "/"
	}
	targetDriverRoot := strings.TrimSpace(opts.HostDriverRoot)
	if targetDriverRoot == "" {
		targetDriverRoot = "/"
	}

	if err := os.MkdirAll(cdiRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create CDI root %q: %w", cdiRoot, err)
	}

	cache, err := cdiapi.NewCache(cdiapi.WithSpecDirs(cdiRoot))
	if err != nil {
		return nil, fmt.Errorf("create CDI cache: %w", err)
	}

	nvmlLib := nvml.New(nvmlLibraryOptions(driverRoot)...)
	deviceLib := nvdevice.New(nvmlLib)
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	nvcdiLib, err := nvcdiapi.New(
		nvcdiapi.WithDeviceLib(deviceLib),
		nvcdiapi.WithDriverRoot(driverRoot),
		nvcdiapi.WithDevRoot(devRootFor(driverRoot)),
		nvcdiapi.WithLogger(logger),
		nvcdiapi.WithNvmlLib(nvmlLib),
		nvcdiapi.WithMode("nvml"),
		nvcdiapi.WithVendor(vendor),
		nvcdiapi.WithClass(class),
		nvcdiapi.WithNVIDIACDIHookPath(strings.TrimSpace(opts.NvidiaCDIHookPath)),
	)
	if err != nil {
		return nil, fmt.Errorf("create nvcdi library: %w", err)
	}

	return &Writer{
		nvml:             nvmlLib,
		nvcdi:            nvcdiLib,
		cache:            cache,
		driverRoot:       driverRoot,
		targetDriverRoot: targetDriverRoot,
		vendor:           vendor,
		class:            class,
	}, nil
}

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

	commonEdits, err := w.commonEdits()
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

	spec, err := buildSpec(w.vendor, w.class, deviceSpecs, commonEdits.ContainerEdits)
	if err != nil {
		return nil, err
	}

	specName := cdiapi.GenerateTransientSpecName(w.vendor, w.class, req.ClaimUID)
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
	specName := cdiapi.GenerateTransientSpecName(w.vendor, w.class, claimUID)
	return w.cache.RemoveSpec(specName)
}

func (w *Writer) initNVML() error {
	ret := w.nvml.Init()
	if ret == nvml.SUCCESS || ret == nvml.ERROR_ALREADY_INITIALIZED {
		return nil
	}
	return fmt.Errorf("NVML init failed: %s", nvml.ErrorString(ret))
}
