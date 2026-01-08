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
	"fmt"
	"io"

	nvdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvcdiapi "github.com/NVIDIA/nvidia-container-toolkit/pkg/nvcdi"
	"github.com/sirupsen/logrus"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
)

func newCDICache(cdiRoot string) (*cdiapi.Cache, error) {
	return cdiapi.NewCache(cdiapi.WithSpecDirs(cdiRoot))
}

func newNVMLLib(driverRoot string) nvml.Interface {
	return nvml.New(nvmlLibraryOptions(driverRoot)...)
}

func newNvcdiInterfaces(params writerParams, deviceLib nvdevice.Interface, nvmlLib nvml.Interface) (nvcdiapi.Interface, nvcdiapi.Interface, error) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	nvcdiDevice, err := nvcdiapi.New(
		nvcdiapi.WithDeviceLib(deviceLib),
		nvcdiapi.WithDriverRoot(params.driverRoot),
		nvcdiapi.WithDevRoot(devRootFor(params.driverRoot)),
		nvcdiapi.WithLogger(logger),
		nvcdiapi.WithNvmlLib(nvmlLib),
		nvcdiapi.WithMode("nvml"),
		nvcdiapi.WithVendor(params.vendor),
		nvcdiapi.WithClass(params.deviceClass),
		nvcdiapi.WithNVIDIACDIHookPath(params.hookPath),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create nvcdi device library: %w", err)
	}

	nvcdiClaim, err := nvcdiapi.New(
		nvcdiapi.WithDeviceLib(deviceLib),
		nvcdiapi.WithDriverRoot(params.driverRoot),
		nvcdiapi.WithDevRoot(devRootFor(params.driverRoot)),
		nvcdiapi.WithLogger(logger),
		nvcdiapi.WithNvmlLib(nvmlLib),
		nvcdiapi.WithMode("nvml"),
		nvcdiapi.WithVendor(params.vendor),
		nvcdiapi.WithClass(params.claimClass),
		nvcdiapi.WithNVIDIACDIHookPath(params.hookPath),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create nvcdi claim library: %w", err)
	}

	return nvcdiDevice, nvcdiClaim, nil
}
