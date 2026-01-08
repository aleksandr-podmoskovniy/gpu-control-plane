//go:build !linux
// +build !linux

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
	"context"
	"errors"

	configapi "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/configapi"
)

// Manager is a no-op time-slicing manager for unsupported platforms.
type Manager struct{}

// Options configure the time-slicing manager.
type Options struct {
	DriverRoot    string
	NvidiaSMIPath string
	NVMLLibPath   string
}

// New constructs a stub time-slicing manager.
func New(_ Options) *Manager {
	return &Manager{}
}

// SetTimeSlice always returns an unsupported error on non-Linux platforms.
func (m *Manager) SetTimeSlice(_ context.Context, _ []string, _ *configapi.TimeSlicingConfig) error {
	return errors.New("time-slicing is not supported on this platform")
}
