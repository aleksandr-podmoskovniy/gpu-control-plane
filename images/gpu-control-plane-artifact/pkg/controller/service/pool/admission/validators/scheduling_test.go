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

package validators

import (
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestSchedulingValidator(t *testing.T) {
	validate := Scheduling()

	t.Run("invalid-strategy", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{Strategy: "Unknown"}}
		if err := validate(spec); err == nil {
			t.Fatalf("expected error for invalid strategy")
		}
	})

	t.Run("spread-without-topology", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{Strategy: v1alpha1.GPUPoolSchedulingSpread}}
		if err := validate(spec); err == nil {
			t.Fatalf("expected error for spread without topology")
		}
	})

	t.Run("spread-valid-trims-taints", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{
				Strategy:    v1alpha1.GPUPoolSchedulingSpread,
				TopologyKey: "topology.kubernetes.io/zone",
				Taints: []v1alpha1.GPUPoolTaintSpec{
					{Key: "k", Value: " v ", Effect: " NoSchedule "},
				},
			},
		}
		if err := validate(spec); err != nil {
			t.Fatalf("expected valid scheduling, got %v", err)
		}
		if spec.Scheduling.Taints[0].Effect != "NoSchedule" {
			t.Fatalf("expected trimmed effect, got %q", spec.Scheduling.Taints[0].Effect)
		}
	})

	t.Run("empty-strategy-valid", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{}
		if err := validate(spec); err != nil {
			t.Fatalf("expected empty scheduling to be valid, got %v", err)
		}
		if spec.Scheduling.TaintsEnabled == nil || !*spec.Scheduling.TaintsEnabled {
			t.Fatalf("expected taintsEnabled to default to true")
		}
	})

	t.Run("binpack-valid", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{Strategy: v1alpha1.GPUPoolSchedulingBinPack}}
		if err := validate(spec); err != nil {
			t.Fatalf("expected binpack scheduling valid, got %v", err)
		}
		if spec.Scheduling.TaintsEnabled == nil || !*spec.Scheduling.TaintsEnabled {
			t.Fatalf("expected taintsEnabled to default to true")
		}
	})

	t.Run("empty-taint-key", func(t *testing.T) {
		spec := &v1alpha1.GPUPoolSpec{
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{
				Strategy: v1alpha1.GPUPoolSchedulingBinPack,
				Taints:   []v1alpha1.GPUPoolTaintSpec{{Key: "  ", Value: "v"}},
			},
		}
		if err := validate(spec); err == nil {
			t.Fatalf("expected empty taint key to error")
		}
	})
}
