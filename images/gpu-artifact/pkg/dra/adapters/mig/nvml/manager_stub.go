//go:build !linux || !cgo || !nvml
// +build !linux !cgo !nvml

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
	"context"
	"errors"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// Manager is a stub for unsupported platforms.
type Manager struct{}

// Options configure the MIG manager.
type Options struct {
	DriverRoot string
}

// New constructs a stub MIG manager.
func New(_ Options) *Manager {
	return &Manager{}
}

// Prepare reports missing NVML support.
func (m *Manager) Prepare(_ context.Context, _ domain.MigPrepareRequest) (domain.PreparedMigDevice, error) {
	return domain.PreparedMigDevice{}, errors.New("MIG support requires linux,cgo,nvml build tags")
}

// Unprepare reports missing NVML support.
func (m *Manager) Unprepare(_ context.Context, _ domain.PreparedMigDevice) error {
	return errors.New("MIG support requires linux,cgo,nvml build tags")
}
