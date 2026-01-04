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

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	domainallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

func TestBuildAllocationResult(t *testing.T) {
	t.Parallel()

	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "claim-1"},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{{
					Name: "gpu",
					Exactly: &resourcev1.ExactDeviceRequest{
						DeviceClassName: "class-a",
					},
				}},
			},
		},
	}

	alloc := &domain.AllocationResult{
		NodeName: "node-1",
		Devices: []domain.AllocatedDevice{{
			Request: "gpu",
			Driver:  domainallocator.DefaultDriverName,
			Pool:    "pool-a",
			Device:  "dev-1",
		}},
	}

	classes := map[string]*resourcev1.DeviceClass{
		"class-a": {ObjectMeta: metav1.ObjectMeta{Name: "class-a"}},
	}
	out, err := BuildAllocationResult(claim, alloc, classes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || len(out.Devices.Results) != 1 {
		t.Fatalf("expected allocation result with 1 device")
	}
	if out.NodeSelector == nil {
		t.Fatalf("expected node selector")
	}
}
