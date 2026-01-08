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
	"os"

	nvdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvcdiapi "github.com/NVIDIA/nvidia-container-toolkit/pkg/nvcdi"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
)

// Writer generates CDI specs using NVIDIA nvcdi.
type Writer struct {
	nvml             nvml.Interface
	nvcdiDevice      nvcdiapi.Interface
	nvcdiClaim       nvcdiapi.Interface
	cache            *cdiapi.Cache
	driverRoot       string
	targetDriverRoot string
	vendor           string
	deviceClass      string
	claimClass       string
}

// New creates a CDI writer backed by nvcdi.
func New(opts Options) (*Writer, error) {
	params, err := normalizeWriterParams(opts)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(params.cdiRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create CDI root %q: %w", params.cdiRoot, err)
	}

	cache, err := newCDICache(params.cdiRoot)
	if err != nil {
		return nil, fmt.Errorf("create CDI cache: %w", err)
	}

	nvmlLib := newNVMLLib(params.driverRoot)
	deviceLib := nvdevice.New(nvmlLib)
	nvcdiDevice, nvcdiClaim, err := newNvcdiInterfaces(params, deviceLib, nvmlLib)
	if err != nil {
		return nil, err
	}

	return &Writer{
		nvml:             nvmlLib,
		nvcdiDevice:      nvcdiDevice,
		nvcdiClaim:       nvcdiClaim,
		cache:            cache,
		driverRoot:       params.driverRoot,
		targetDriverRoot: params.targetDriverRoot,
		vendor:           params.vendor,
		deviceClass:      params.deviceClass,
		claimClass:       params.claimClass,
	}, nil
}
