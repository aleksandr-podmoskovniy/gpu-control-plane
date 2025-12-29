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

package k8s

import (
	"context"

	resourcev1 "k8s.io/api/resource/v1"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// AllocationWriter persists allocation results (no-op for now).
type AllocationWriter struct{}

// NewAllocationWriter creates a new allocation writer.
func NewAllocationWriter() *AllocationWriter {
	return &AllocationWriter{}
}

// Write implements ports.AllocationWriter.
func (w *AllocationWriter) Write(_ context.Context, _ domain.AllocationResult) error {
	return nil
}

// ResourceSliceWriter publishes ResourceSlices (no-op for now).
type ResourceSliceWriter struct{}

// NewResourceSliceWriter creates a new ResourceSlice writer.
func NewResourceSliceWriter() *ResourceSliceWriter {
	return &ResourceSliceWriter{}
}

// Publish implements ports.ResourceSliceWriter.
func (w *ResourceSliceWriter) Publish(_ context.Context, _ *resourcev1.ResourceSlice) error {
	return nil
}
