// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package allocator

import (
	"context"
	"errors"
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

type fakeInventory struct {
	snapshot domain.InventorySnapshot
	err      error
	calls    int
}

func (f *fakeInventory) Snapshot(_ context.Context) (domain.InventorySnapshot, error) {
	f.calls++
	return f.snapshot, f.err
}

type fakeWriter struct {
	result domain.AllocationResult
	err    error
	calls  int
}

func (f *fakeWriter) Write(_ context.Context, result domain.AllocationResult) error {
	f.calls++
	f.result = result
	return f.err
}

func TestServiceRunOnceWritesAllocation(t *testing.T) {
	t.Parallel()

	inventory := &fakeInventory{snapshot: domain.InventorySnapshot{NodeName: "node-1"}}
	writer := &fakeWriter{}
	service := NewService(inventory, writer)

	if err := service.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if inventory.calls != 1 {
		t.Fatalf("expected inventory calls=1, got %d", inventory.calls)
	}
	if writer.calls != 1 {
		t.Fatalf("expected writer calls=1, got %d", writer.calls)
	}
	if writer.result.NodeName != "node-1" {
		t.Fatalf("expected NodeName=node-1, got %q", writer.result.NodeName)
	}
}

func TestServiceRunOnceInventoryError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("boom")
	inventory := &fakeInventory{err: expectedErr}
	writer := &fakeWriter{}
	service := NewService(inventory, writer)

	if err := service.RunOnce(context.Background()); !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if writer.calls != 0 {
		t.Fatalf("expected writer not called, got %d", writer.calls)
	}
}
