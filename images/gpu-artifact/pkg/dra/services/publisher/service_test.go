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

package publisher

import (
	"context"
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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
	slice *resourcev1.ResourceSlice
	err   error
	calls int
}

func (f *fakeWriter) Publish(_ context.Context, slice *resourcev1.ResourceSlice) error {
	f.calls++
	f.slice = slice
	return f.err
}

func TestPublishOnceNodeSlice(t *testing.T) {
	t.Parallel()

	inventory := &fakeInventory{snapshot: domain.InventorySnapshot{NodeName: "node-a", NodeUID: "uid-1"}}
	writer := &fakeWriter{}
	service := NewService(inventory, writer)

	if err := service.PublishOnce(context.Background(), false); err != nil {
		t.Fatalf("PublishOnce returned error: %v", err)
	}
	if writer.calls != 1 {
		t.Fatalf("expected writer calls=1, got %d", writer.calls)
	}

	slice := writer.slice
	if slice.Name != "gpu-node-a" {
		t.Fatalf("expected name gpu-node-a, got %q", slice.Name)
	}
	if slice.Spec.NodeName == nil || *slice.Spec.NodeName != "node-a" {
		t.Fatalf("expected spec.nodeName=node-a, got %v", slice.Spec.NodeName)
	}
	if slice.Spec.AllNodes != nil {
		t.Fatalf("expected spec.allNodes nil, got %v", slice.Spec.AllNodes)
	}
	if slice.Annotations[annotationKey] != driverName {
		t.Fatalf("expected annotation %s=%s, got %v", annotationKey, driverName, slice.Annotations)
	}
	if !controllerutil.ContainsFinalizer(slice, finalizerName) {
		t.Fatalf("expected finalizer %q", finalizerName)
	}
	if len(slice.OwnerReferences) != 1 {
		t.Fatalf("expected one owner reference, got %d", len(slice.OwnerReferences))
	}
}

func TestPublishOnceAllNodesSlice(t *testing.T) {
	t.Parallel()

	inventory := &fakeInventory{snapshot: domain.InventorySnapshot{}}
	writer := &fakeWriter{}
	service := NewService(inventory, writer)

	if err := service.PublishOnce(context.Background(), false); err != nil {
		t.Fatalf("PublishOnce returned error: %v", err)
	}

	slice := writer.slice
	if slice.Name != "gpu-unknown" {
		t.Fatalf("expected name gpu-unknown, got %q", slice.Name)
	}
	if slice.Spec.NodeName != nil {
		t.Fatalf("expected spec.nodeName nil, got %v", slice.Spec.NodeName)
	}
	if slice.Spec.AllNodes == nil || !*slice.Spec.AllNodes {
		t.Fatalf("expected spec.allNodes true, got %v", slice.Spec.AllNodes)
	}
	if len(slice.OwnerReferences) != 0 {
		t.Fatalf("expected no owner references, got %d", len(slice.OwnerReferences))
	}
}
