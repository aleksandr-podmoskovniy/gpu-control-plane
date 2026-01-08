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

package nvml

import (
	"context"
	"errors"
)

// Checker reports missing NVML support.
type Checker struct{}

// Options configures the NVML checker.
type Options struct{}

// NewChecker constructs a stub checker for unsupported builds.
func NewChecker(_ Options) *Checker {
	return &Checker{}
}

// EnsureGPUFree always reports missing NVML support.
func (c *Checker) EnsureGPUFree(_ context.Context, _ string) error {
	return errors.New("nvml support is required for gpu free checks")
}
