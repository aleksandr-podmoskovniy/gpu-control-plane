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

package allocator

import (
	"context"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

// DeviceKey identifies a device in allocation results.
type DeviceKey struct {
	Driver string
	Pool   string
	Device string
}

// Selector matches a device against a request.
type Selector interface {
	Match(ctx context.Context, driver string, spec allocatable.DeviceSpec) (bool, error)
}

// Request represents a single device request.
type Request struct {
	Name      string
	Count     int64
	Selectors []Selector
}

// CandidateDevice represents a device offer for allocation.
type CandidateDevice struct {
	Key      DeviceKey
	Driver   string
	Pool     string
	NodeName string
	Spec     allocatable.DeviceSpec
}

// Input provides data needed to allocate devices for a claim.
type Input struct {
	Requests   []Request
	Candidates []CandidateDevice
}
