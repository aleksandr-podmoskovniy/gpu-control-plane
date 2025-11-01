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

package admission

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/go-logr/logr/testr"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestPoolSnapshotHandler(t *testing.T) {
	h := NewPoolSnapshotHandler(testr.New(t))
	pool := &gpuv1alpha1.GPUPool{}
	pool.Status.Capacity.Total = 5

	if _, err := h.SyncPool(context.Background(), pool); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	value, ok := pool.Annotations["gpu.deckhouse.io/pool-status"]
	if !ok {
		t.Fatal("snapshot annotation missing")
	}

	var status gpuv1alpha1.GPUPoolStatus
	if err := json.Unmarshal([]byte(value), &status); err != nil {
		t.Fatalf("annotation not valid json: %v", err)
	}
	if status.Capacity.Total != 5 {
		t.Fatalf("unexpected capacity total: %d", status.Capacity.Total)
	}
}

func TestPoolSnapshotHandlerName(t *testing.T) {
	if NewPoolSnapshotHandler(testr.New(t)).Name() != "pool-snapshot" {
		t.Fatalf("unexpected handler name")
	}
}

func TestPoolSnapshotHandlerPreservesExistingAnnotations(t *testing.T) {
	h := NewPoolSnapshotHandler(testr.New(t))
	pool := &gpuv1alpha1.GPUPool{}
	pool.Annotations = map[string]string{"existing": "value"}

	if _, err := h.SyncPool(context.Background(), pool); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool.Annotations["existing"] != "value" {
		t.Fatalf("existing annotation should remain untouched, got %+v", pool.Annotations)
	}
	if _, ok := pool.Annotations["gpu.deckhouse.io/pool-status"]; !ok {
		t.Fatal("expected pool status annotation to be populated")
	}
}

func TestPoolSnapshotHandlerMarshalError(t *testing.T) {
	orig := poolStatusMarshal
	defer func() { poolStatusMarshal = orig }()
	poolStatusMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal fail") }

	h := NewPoolSnapshotHandler(testr.New(t))
	if _, err := h.SyncPool(context.Background(), &gpuv1alpha1.GPUPool{}); err == nil {
		t.Fatal("expected marshal error")
	}
}
