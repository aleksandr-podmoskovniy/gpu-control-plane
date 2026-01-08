//go:build linux
// +build linux

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

package vfio

import (
	"errors"
	"fmt"
	"os"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
)

const (
	vfioRoot = "/dev/vfio"
)

// CDIWriter generates CDI specs for vfio-pci devices.
type CDIWriter struct {
	vendor    string
	class     string
	cdiRoot   string
	sysfsRoot string
	cache     *cdiapi.Cache
}

// CDIOptions configure the VFIO CDI writer.
type CDIOptions struct {
	Vendor    string
	Class     string
	CDIRoot   string
	SysfsRoot string
}

// NewCDIWriter constructs a VFIO CDI writer.
func NewCDIWriter(opts CDIOptions) (*CDIWriter, error) {
	vendor := opts.Vendor
	if vendor == "" {
		return nil, errors.New("vendor is required")
	}
	class := opts.Class
	if class == "" {
		return nil, errors.New("class is required")
	}
	cdiRoot := opts.CDIRoot
	if cdiRoot == "" {
		cdiRoot = "/etc/cdi"
	}
	sysfsRoot := opts.SysfsRoot
	if sysfsRoot == "" {
		sysfsRoot = defaultSysfsRoot
	}
	if err := os.MkdirAll(cdiRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create CDI root %q: %w", cdiRoot, err)
	}
	cache, err := cdiapi.NewCache(cdiapi.WithSpecDirs(cdiRoot))
	if err != nil {
		return nil, fmt.Errorf("create CDI cache: %w", err)
	}
	return &CDIWriter{
		vendor:    vendor,
		class:     class,
		cdiRoot:   cdiRoot,
		sysfsRoot: sysfsRoot,
		cache:     cache,
	}, nil
}
