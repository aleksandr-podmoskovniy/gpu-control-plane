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

func TestGPUNodeStateStatusDeepCopy(t *testing.T) {
	ts := metav1.NewTime(time.Unix(1710000000, 0))

	original := &GPUNodeStateStatus{
		Conditions: []metav1.Condition{{
			Type:               "ReadyForPooling",
			Status:             metav1.ConditionTrue,
			Reason:             "OK",
			Message:            "node ready",
			LastTransitionTime: ts,
		}},
	}

	cloned := original.DeepCopy()
	if cloned == original {
		t.Fatal("expected deep copy to allocate a new instance")
	}

	cloned.Conditions[0].Reason = "Changed"
	if original.Conditions[0].Reason != "OK" {
		t.Fatal("conditions slice should be deep-copied")
	}
}
