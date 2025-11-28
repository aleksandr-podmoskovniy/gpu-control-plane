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
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterGPUPoolDeepCopy(t *testing.T) {
	count := int32(1)
	taintsEnabled := true
	src := &ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "cluster-pool",
			Labels: map[string]string{"a": "b"},
		},
		Spec: GPUPoolSpec{
			Provider: "Nvidia",
			Backend:  "DevicePlugin",
			Resource: GPUPoolResourceSpec{
				Unit:          "MIG",
				SlicesPerUnit: 2,
				MIGLayout: []GPUPoolMIGDeviceLayout{{
					Profiles: []GPUPoolMIGProfile{{Name: "1g.10gb", Count: &count}},
				}},
			},
			NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"node": "gpu"}},
			DeviceSelector: &GPUPoolDeviceSelector{
				Include: GPUPoolSelectorRules{PCIVendors: []string{"10de"}, MIGProfiles: []string{"1g.10gb"}},
			},
			Scheduling: GPUPoolSchedulingSpec{TaintsEnabled: &taintsEnabled},
		},
		Status: GPUPoolStatus{
			Capacity: GPUPoolCapacityStatus{Total: 1, Available: 1, SlicesPerUnit: 2},
		},
	}

	cp := src.DeepCopy()
	if cp == src {
		t.Fatalf("DeepCopy must return new pointer")
	}
	if !reflect.DeepEqual(cp.Spec, src.Spec) {
		t.Fatalf("spec must be equal after deep copy")
	}
	cp.Spec.Resource.SlicesPerUnit = 10
	if src.Spec.Resource.SlicesPerUnit == 10 {
		t.Fatalf("deep copy must not mutate source")
	}
}
