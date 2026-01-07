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

package vfio

import (
	"context"
	"errors"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// CDIWriter is a stub for unsupported platforms.
type CDIWriter struct{}

// CDIOptions configure the VFIO CDI writer.
type CDIOptions struct {
	Vendor    string
	Class     string
	CDIRoot   string
	SysfsRoot string
}

// NewCDIWriter constructs a stub VFIO CDI writer.
func NewCDIWriter(_ CDIOptions) (*CDIWriter, error) {
	return nil, errors.New("vfio CDI requires linux")
}

// Write reports missing support.
func (w *CDIWriter) Write(_ context.Context, _ domain.PrepareRequest) (map[string][]string, error) {
	return nil, errors.New("vfio CDI requires linux")
}

// Delete reports missing support.
func (w *CDIWriter) Delete(_ context.Context, _ string) error {
	return errors.New("vfio CDI requires linux")
}
