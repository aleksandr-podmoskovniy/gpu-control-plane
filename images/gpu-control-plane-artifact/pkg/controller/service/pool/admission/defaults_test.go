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
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestApplyDefaults(t *testing.T) {
	spec := &v1alpha1.GPUPoolSpec{}
	applyDefaults(spec)
	if spec.Provider != defaultProvider || spec.Backend != defaultBackend {
		t.Fatalf("unexpected defaults: %+v", spec)
	}
	if spec.Resource.SlicesPerUnit != 1 {
		t.Fatalf("expected slicesPerUnit=1, got %d", spec.Resource.SlicesPerUnit)
	}
	if spec.Scheduling.Strategy != v1alpha1.GPUPoolSchedulingSpread || spec.Scheduling.TopologyKey == "" {
		t.Fatalf("unexpected scheduling defaults: %+v", spec.Scheduling)
	}

	spec = &v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{Strategy: v1alpha1.GPUPoolSchedulingBinPack}}
	applyDefaults(spec)
	if spec.Scheduling.TopologyKey != "" {
		t.Fatalf("topologyKey must not be forced for BinPack, got %q", spec.Scheduling.TopologyKey)
	}
}
