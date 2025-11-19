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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGPUNodeBootstrapStatusDeepCopy(t *testing.T) {
	ts := metav1.NewTime(time.Unix(1710000000, 0))

	original := &GPUNodeBootstrapStatus{
		Phase:        GPUNodeBootstrapPhaseMonitoring,
		Components:   map[string]bool{"validator": true},
		GFDReady:     true,
		ToolkitReady: true,
		LastRun:      &ts,
		Workloads: []GPUNodeBootstrapWorkloadStatus{{
			Name:    "validator",
			Healthy: true,
			Since:   &ts,
			Message: "ok",
		}},
		PendingDevices: []string{"devA", "devB"},
		Validations: []GPUNodeValidationState{{
			InventoryID: "devA",
			Attempts:    2,
			LastFailure: &ts,
		}},
	}

	cloned := original.DeepCopy()
	if cloned == original {
		t.Fatal("expected deep copy to allocate a new instance")
	}
	if cloned.LastRun == original.LastRun || cloned.Workloads[0].Since == original.Workloads[0].Since {
		t.Fatal("expected timestamps to be copied")
	}
	if cloned.Components["validator"] != original.Components["validator"] {
		t.Fatal("components map corrupted")
	}

	cloned.Components["validator"] = false
	cloned.PendingDevices[0] = "mutated"
	cloned.Validations[0].Attempts = 5
	if original.Components["validator"] != true {
		t.Fatal("mutating clone should not affect original")
	}
	if original.PendingDevices[0] != "devA" {
		t.Fatal("pending devices should be copied by value")
	}
	if original.Validations[0].Attempts != 2 {
		t.Fatal("validations slice should be copied")
	}

	workload := &GPUNodeBootstrapWorkloadStatus{Since: &ts}
	if workload.DeepCopy() == workload {
		t.Fatal("expected workload deepcopy to clone struct")
	}
	var nilWorkload *GPUNodeBootstrapWorkloadStatus
	if nilWorkload.DeepCopy() != nil {
		t.Fatal("nil workload deepcopy should return nil")
	}

	validation := &GPUNodeValidationState{LastFailure: &ts}
	if validation.DeepCopy() == validation {
		t.Fatal("expected validation deepcopy to clone struct")
	}
	var nilValidation *GPUNodeValidationState
	if nilValidation.DeepCopy() != nil {
		t.Fatal("nil validation deepcopy should return nil")
	}
}
