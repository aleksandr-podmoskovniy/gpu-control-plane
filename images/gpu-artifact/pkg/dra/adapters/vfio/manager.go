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

const (
	defaultSysfsRoot = "/sys/bus/pci/devices"
	driversRoot      = "/sys/bus/pci/drivers"
	vfioDriver       = "vfio-pci"
	defaultDriver    = "nvidia"
)

// Manager binds devices to vfio-pci.
type Manager struct {
	sysfsRoot string
}

// Options configure the VFIO manager.
type Options struct {
	SysfsRoot string
}

// New constructs a VFIO manager.
func New(opts Options) *Manager {
	root := opts.SysfsRoot
	if root == "" {
		root = defaultSysfsRoot
	}
	return &Manager{sysfsRoot: root}
}
