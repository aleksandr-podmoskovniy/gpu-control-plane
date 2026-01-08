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

package mps

import (
	"context"
	"errors"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// Manager is a no-op MPS manager for unsupported platforms.
type Manager struct{}

// Options configure the MPS manager.
type Options struct {
	PluginPath     string
	DriverRoot     string
	ControlBinPath string
	ShmDir         string
}

// New constructs a stub MPS manager.
func New(_ Options) *Manager {
	return &Manager{}
}

// Start always returns an unsupported error on non-Linux platforms.
func (m *Manager) Start(_ context.Context, _ domain.MpsPrepareRequest) (domain.PreparedMpsState, error) {
	return domain.PreparedMpsState{}, errors.New("mps is not supported on this platform")
}

// Stop always returns an unsupported error on non-Linux platforms.
func (m *Manager) Stop(_ context.Context, _ domain.PreparedMpsState) error {
	return errors.New("mps is not supported on this platform")
}
