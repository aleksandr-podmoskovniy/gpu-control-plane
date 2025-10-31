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

package gpupool

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestCapacitySyncHandler(t *testing.T) {
	h := NewCapacitySyncHandler(testr.New(t))
	pool := &gpuv1alpha1.GPUPool{}
	pool.Status.Nodes = []gpuv1alpha1.GPUPoolNodeStatus{{TotalDevices: 3, AssignedDevices: 1}, {TotalDevices: 2, AssignedDevices: 2}}

	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pool.Status.Capacity.Total != 5 || pool.Status.Capacity.Used != 3 || pool.Status.Capacity.Available != 2 {
		t.Fatalf("unexpected capacity: %+v", pool.Status.Capacity)
	}
}
