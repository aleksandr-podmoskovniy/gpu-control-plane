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

package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGPUPoolDeepCopyLocal(t *testing.T) {
	pool := &GPUPool{
		Spec: GPUPoolSpec{
			Resource:       GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 1},
			Scheduling:     GPUPoolSchedulingSpec{Strategy: GPUPoolSchedulingSpread},
			DeviceSelector: &GPUPoolDeviceSelector{Include: GPUPoolSelectorRules{PCIVendors: []string{"10de"}}},
			DeviceAssignment: GPUPoolAssignmentSpec{
				AutoApproveSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
			},
		},
	}
	copy := pool.DeepCopy()
	if copy == pool {
		t.Fatalf("pool deepcopy must allocate new struct")
	}
	copy.Spec.Resource.SlicesPerUnit = 10
	if pool.Spec.Resource.SlicesPerUnit == 10 {
		t.Fatalf("mutation of copy must not affect source")
	}
	copy.Spec.DeviceAssignment.AutoApproveSelector.MatchLabels["a"] = "c"
	if pool.Spec.DeviceAssignment.AutoApproveSelector.MatchLabels["a"] != "b" {
		t.Fatalf("label selector must be deep-copied")
	}
}
