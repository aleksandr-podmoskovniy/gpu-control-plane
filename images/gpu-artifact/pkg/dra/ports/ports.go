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

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
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
	Publish(ctx context.Context, nodeName string, inventory allocatable.Inventory) error
}

// PrepareLocker serializes prepare/unprepare calls on a node.
type PrepareLocker interface {
	Lock(ctx context.Context) (func() error, error)
}

// PrepareCheckpointStore persists prepare/unprepare state.
type PrepareCheckpointStore interface {
	Load(ctx context.Context) (domain.PrepareCheckpoint, error)
	Save(ctx context.Context, checkpoint domain.PrepareCheckpoint) error
}

// MigManager creates and deletes MIG instances.
type MigManager interface {
	Prepare(ctx context.Context, req domain.MigPrepareRequest) (domain.PreparedMigDevice, error)
	Unprepare(ctx context.Context, state domain.PreparedMigDevice) error
}

// VfioManager binds and unbinds PCI devices to vfio-pci.
type VfioManager interface {
	Prepare(ctx context.Context, req domain.VfioPrepareRequest) (domain.PreparedVfioDevice, error)
	Unprepare(ctx context.Context, state domain.PreparedVfioDevice) error
}

// CDIWriter writes CDI specifications for prepared devices.
type CDIWriter interface {
	Write(ctx context.Context, req domain.PrepareRequest) (map[string][]string, error)
	Delete(ctx context.Context, claimUID string) error
}

// HookWriter writes VM hook payloads when required.
type HookWriter interface {
	Write(ctx context.Context, req domain.PrepareRequest) error
}
