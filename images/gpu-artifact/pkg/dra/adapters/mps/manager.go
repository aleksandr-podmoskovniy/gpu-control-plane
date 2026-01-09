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

package mps

import (
	"os/exec"
	"strings"
)

const (
	defaultControlBinary = "nvidia-cuda-mps-control"
	defaultControlDir    = "mps"
	defaultPipeDir       = "pipe"
	defaultLogDir        = "log"
	defaultShmDir        = "/dev/shm"
	startupLogName       = "startup.log"
)

var controlRelDirs = []string{
	"usr/bin",
	"usr/sbin",
	"bin",
	"sbin",
}

// Manager starts and stops CUDA MPS control daemons.
type Manager struct {
	controlBinary string
	pluginPath    string
	driverRoot    string
	shmDir        string
}

// Options configure the MPS manager.
type Options struct {
	PluginPath     string
	DriverRoot     string
	ControlBinPath string
	ShmDir         string
}

// New constructs an MPS manager.
func New(opts Options) *Manager {
	controlPath := strings.TrimSpace(opts.ControlBinPath)
	if controlPath == "" {
		controlPath = resolveBinary(opts.DriverRoot, controlRelDirs, defaultControlBinary)
		if controlPath == "" {
			if resolved, err := exec.LookPath(defaultControlBinary); err == nil {
				controlPath = resolved
			}
		}
	}
	shmDir := strings.TrimSpace(opts.ShmDir)
	if shmDir == "" {
		shmDir = defaultShmDir
	}
	return &Manager{
		controlBinary: controlPath,
		pluginPath:    strings.TrimSpace(opts.PluginPath),
		driverRoot:    strings.TrimSpace(opts.DriverRoot),
		shmDir:        shmDir,
	}
}
