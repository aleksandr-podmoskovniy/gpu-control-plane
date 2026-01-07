/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package prepare

import (
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func TestBuildPrepareRequest(t *testing.T) {
	claimUID := types.UID("claim-uid")
	shareID := types.UID("share-1")

	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "claim-1",
			UID:  claimUID,
		},
		Status: resourcev1.ResourceClaimStatus{
			Allocation: &resourcev1.AllocationResult{
				Devices: resourcev1.DeviceAllocationResult{
					Results: []resourcev1.DeviceRequestAllocationResult{
						{
							Request: "req-1",
							Driver:  "gpu.deckhouse.io",
							Pool:    "pool-a",
							Device:  "gpu-1",
							ShareID: &shareID,
							ConsumedCapacity: map[resourcev1.QualifiedName]resource.Quantity{
								resourcev1.QualifiedName(allocatable.CapSharePercent): resource.MustParse("50"),
							},
						},
					},
				},
			},
		},
	}

	slices := []resourcev1.ResourceSlice{
		{
			Spec: resourcev1.ResourceSliceSpec{
				Driver: "gpu.deckhouse.io",
				Pool: resourcev1.ResourcePool{
					Name:       "pool-a",
					Generation: 1,
				},
				NodeName: ptr.To("node-1"),
				Devices: []resourcev1.Device{
					{
						Name: "gpu-old",
					},
				},
			},
		},
		{
			Spec: resourcev1.ResourceSliceSpec{
				Driver: "gpu.deckhouse.io",
				Pool: resourcev1.ResourcePool{
					Name:       "pool-a",
					Generation: 2,
				},
				NodeName: ptr.To("node-1"),
				Devices: []resourcev1.Device{
					{
						Name: "gpu-1",
						Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
							resourcev1.QualifiedName(allocatable.AttrGPUUUID): {
								StringValue: ptr.To("uuid-1"),
							},
							resourcev1.QualifiedName(allocatable.AttrDeviceType): {
								StringValue: ptr.To("Physical"),
							},
						},
					},
				},
			},
		},
	}

	req, err := BuildPrepareRequest(claim, "gpu.deckhouse.io", "node-1", slices)
	if err != nil {
		t.Fatalf("BuildPrepareRequest error: %v", err)
	}
	if req.ClaimUID != string(claimUID) {
		t.Fatalf("unexpected claim UID: %q", req.ClaimUID)
	}
	if len(req.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(req.Devices))
	}
	dev := req.Devices[0]
	if dev.Device != "gpu-1" {
		t.Fatalf("unexpected device name: %q", dev.Device)
	}
	if dev.ShareID != "share-1" {
		t.Fatalf("unexpected share ID: %q", dev.ShareID)
	}
	if dev.Attributes[allocatable.AttrGPUUUID].String == nil || *dev.Attributes[allocatable.AttrGPUUUID].String != "uuid-1" {
		t.Fatalf("missing gpu uuid attribute")
	}
	if dev.ConsumedCapacity == nil {
		t.Fatalf("expected consumed capacity")
	}
	if got := dev.ConsumedCapacity[allocatable.CapSharePercent]; got.String() != "50" {
		t.Fatalf("unexpected share percent: %s", got.String())
	}
}
