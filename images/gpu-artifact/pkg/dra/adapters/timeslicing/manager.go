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

package timeslicing

import (
	"os/exec"
	"strings"
)

const (
	defaultSMIBinary   = "nvidia-smi"
	defaultNVMLLib     = "libnvidia-ml.so.1"
	defaultComputeMode = "DEFAULT"
)

var smiRelDirs = []string{
	"opt/bin",
	"usr/bin",
	"usr/sbin",
	"bin",
	"sbin",
}

var nvmlRelDirs = []string{
	"usr/lib64",
	"usr/lib/x86_64-linux-gnu",
	"usr/lib/aarch64-linux-gnu",
	"lib64",
	"lib/x86_64-linux-gnu",
	"lib/aarch64-linux-gnu",
}

// Manager applies time-slicing settings through nvidia-smi.
type Manager struct {
	nvidiaSMIPath string
	nvmlLibPath   string
}

// Options configure the time-slicing manager.
type Options struct {
	DriverRoot    string
	NvidiaSMIPath string
	NVMLLibPath   string
}

// New constructs a time-slicing manager.
func New(opts Options) *Manager {
	smiPath := strings.TrimSpace(opts.NvidiaSMIPath)
	if smiPath == "" {
		smiPath = resolveBinary(opts.DriverRoot, smiRelDirs, defaultSMIBinary)
		if smiPath == "" {
			if resolved, err := exec.LookPath(defaultSMIBinary); err == nil {
				smiPath = resolved
			}
		}
	}
	nvmlPath := strings.TrimSpace(opts.NVMLLibPath)
	if nvmlPath == "" {
		nvmlPath = resolveBinary(opts.DriverRoot, nvmlRelDirs, defaultNVMLLib)
	}
	return &Manager{
		nvidiaSMIPath: smiPath,
		nvmlLibPath:   nvmlPath,
	}
}
