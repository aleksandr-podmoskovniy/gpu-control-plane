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
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	domainalloc "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	domainallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

func TestBuildCandidatesReturnsDeviceSpec(t *testing.T) {
	t.Parallel()

	slices := []resourcev1.ResourceSlice{resourceSlice("slice-1", "node-1", "pool-a", "dev-1")}
	candidates := BuildCandidates(domainallocator.DefaultDriverName, slices)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Spec.Name != "dev-1" {
		t.Fatalf("unexpected device name: %q", candidates[0].Spec.Name)
	}
}

func TestBuildCounterSets(t *testing.T) {
	t.Parallel()

	slices := []resourcev1.ResourceSlice{
		resourceSliceWithCounters("slice-1", "node-1", "pool-a", "counter-a", resource.MustParse("100Mi")),
		resourceSliceWithCounters("slice-2", "node-2", "pool-b", "counter-b", resource.MustParse("200Mi")),
	}

	sets := BuildCounterSets(domainallocator.DefaultDriverName, slices)
	if len(sets) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(sets))
	}
	if len(sets["node-1"]) != 1 {
		t.Fatalf("expected counter sets for node-1")
	}
	counter := sets["node-1"]["counter-a"].Counters["memory"]
	if counter.Value != 100 || counter.Unit != domainalloc.CounterUnitMiB {
		t.Fatalf("unexpected counter value: %#v", counter)
	}
}

func resourceSlice(name, nodeName, poolName, deviceName string) resourcev1.ResourceSlice {
	return resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: resourcev1.ResourceSliceSpec{
			Driver: domainallocator.DefaultDriverName,
			Pool: resourcev1.ResourcePool{
				Name:               poolName,
				Generation:         1,
				ResourceSliceCount: 1,
			},
			NodeName: ptr.To(nodeName),
			Devices: []resourcev1.Device{{
				Name: deviceName,
			}},
		},
	}
}

func resourceSliceWithCounters(name, nodeName, poolName, counterSetName string, memory resource.Quantity) resourcev1.ResourceSlice {
	slice := resourceSlice(name, nodeName, poolName, "dev-1")
	slice.Spec.SharedCounters = []resourcev1.CounterSet{{
		Name: counterSetName,
		Counters: map[string]resourcev1.Counter{
			"memory": {Value: memory},
		},
	}}
	return slice
}
