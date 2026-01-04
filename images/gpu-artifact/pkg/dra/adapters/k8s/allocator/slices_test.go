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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	domainallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

func TestBuildCandidatesFiltersAllocated(t *testing.T) {
	t.Parallel()

	slices := []resourcev1.ResourceSlice{resourceSlice("slice-1", "node-1", "pool-a", "dev-1")}
	allocated := map[domainallocator.DeviceKey]struct{}{
		{Driver: domainallocator.DefaultDriverName, Pool: "pool-a", Device: "dev-1"}: {},
	}

	candidates := BuildCandidates(domainallocator.DefaultDriverName, slices, allocated)
	if len(candidates) != 0 {
		t.Fatalf("expected no candidates, got %d", len(candidates))
	}
}

func TestBuildCandidatesReturnsDeviceSpec(t *testing.T) {
	t.Parallel()

	slices := []resourcev1.ResourceSlice{resourceSlice("slice-1", "node-1", "pool-a", "dev-1")}
	candidates := BuildCandidates(domainallocator.DefaultDriverName, slices, map[domainallocator.DeviceKey]struct{}{})
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Spec.Name != "dev-1" {
		t.Fatalf("unexpected device name: %q", candidates[0].Spec.Name)
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
