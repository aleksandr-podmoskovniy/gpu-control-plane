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

package nvcdi

import (
	"context"
	"errors"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// Writer is a stub CDI writer for non-NVML builds.
type Writer struct{}

// BaseDevice is a stubbed base device descriptor.
type BaseDevice struct {
	Name string
	UUID string
}

// New returns an error when NVML is unavailable.
func New(_ Options) (*Writer, error) {
	return nil, errors.New("nvcdi requires linux,cgo,nvml build tags")
}

// Write returns an error when NVML is unavailable.
func (w *Writer) Write(_ context.Context, _ domain.PrepareRequest) (map[string][]string, error) {
	return nil, errors.New("nvcdi requires linux,cgo,nvml build tags")
}

// Delete returns an error when NVML is unavailable.
func (w *Writer) Delete(_ context.Context, _ string) error {
	return errors.New("nvcdi requires linux,cgo,nvml build tags")
}

// WriteBase returns an error when NVML is unavailable.
func (w *Writer) WriteBase(_ context.Context, _ []BaseDevice) error {
	return errors.New("nvcdi requires linux,cgo,nvml build tags")
}
