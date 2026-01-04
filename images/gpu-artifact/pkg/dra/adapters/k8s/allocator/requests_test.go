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
)

func TestBuildRequestsExactCount(t *testing.T) {
	t.Parallel()

	claim := claimWithExact("claim-1", "class-a", 2)
	classes := map[string]*resourcev1.DeviceClass{"class-a": deviceClass("class-a")}

	reqs, err := BuildRequests(claim, classes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].Count != 2 || reqs[0].Name != "gpu" {
		t.Fatalf("unexpected request: %+v", reqs[0])
	}
}

func TestBuildRequestsFirstAvailableUnsupported(t *testing.T) {
	t.Parallel()

	claim := &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "claim-1"},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{{
					Name: "gpu",
					FirstAvailable: []resourcev1.DeviceSubRequest{{
						DeviceClassName: "class-a",
					}},
				}},
			},
		},
	}

	_, err := BuildRequests(claim, map[string]*resourcev1.DeviceClass{"class-a": deviceClass("class-a")})
	if err == nil {
		t.Fatalf("expected error for firstAvailable requests")
	}
}

func TestBuildRequestsMissingClass(t *testing.T) {
	t.Parallel()

	claim := claimWithExact("claim-1", "class-a", 1)
	_, err := BuildRequests(claim, map[string]*resourcev1.DeviceClass{})
	if err == nil {
		t.Fatalf("expected error for missing deviceclass")
	}
}

func claimWithExact(name, className string, count int64) *resourcev1.ResourceClaim {
	return &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{{
					Name: "gpu",
					Exactly: &resourcev1.ExactDeviceRequest{
						DeviceClassName: className,
						Count:           count,
						AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
					},
				}},
			},
		},
	}
}

func deviceClass(name string) *resourcev1.DeviceClass {
	return &resourcev1.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       resourcev1.DeviceClassSpec{},
	}
}
