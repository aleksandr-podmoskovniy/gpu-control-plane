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
	"errors"
	"fmt"
	"strings"
)

type writerParams struct {
	vendor           string
	claimClass       string
	deviceClass      string
	cdiRoot          string
	driverRoot       string
	targetDriverRoot string
	hookPath         string
}

func normalizeWriterParams(opts Options) (writerParams, error) {
	vendor := strings.TrimSpace(opts.Vendor)
	if vendor == "" && opts.DriverName != "" {
		vendor = fmt.Sprintf("k8s.%s", opts.DriverName)
	}
	if vendor == "" {
		return writerParams{}, errors.New("vendor is required")
	}
	claimClass := strings.TrimSpace(opts.Class)
	if claimClass == "" {
		claimClass = defaultClaimClass
	}
	deviceClass := strings.TrimSpace(opts.DeviceClass)
	if deviceClass == "" {
		deviceClass = defaultDeviceClass
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
	hookPath := strings.TrimSpace(opts.NvidiaCDIHookPath)
	return writerParams{
		vendor:           vendor,
		claimClass:       claimClass,
		deviceClass:      deviceClass,
		cdiRoot:          cdiRoot,
		driverRoot:       driverRoot,
		targetDriverRoot: targetDriverRoot,
		hookPath:         hookPath,
	}, nil
}
