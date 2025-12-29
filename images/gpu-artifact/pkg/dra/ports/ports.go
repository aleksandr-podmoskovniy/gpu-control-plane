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

package ports

import (
	"context"

	resourcev1 "k8s.io/api/resource/v1"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// InventoryProvider returns a node inventory snapshot.
type InventoryProvider interface {
	Snapshot(ctx context.Context) (domain.InventorySnapshot, error)
}

// AllocationWriter persists allocation results.
type AllocationWriter interface {
	Write(ctx context.Context, result domain.AllocationResult) error
}

// ResourceSliceWriter publishes ResourceSlices.
type ResourceSliceWriter interface {
	Publish(ctx context.Context, slice *resourcev1.ResourceSlice) error
}

// CheckpointStore persists prepare/unprepare state.
type CheckpointStore interface {
	Save(ctx context.Context, req domain.PrepareRequest) error
}

// CDIWriter writes CDI specifications for prepared devices.
type CDIWriter interface {
	Write(ctx context.Context, req domain.PrepareRequest) error
}

// HookWriter writes VM hook payloads when required.
type HookWriter interface {
	Write(ctx context.Context, req domain.PrepareRequest) error
}
