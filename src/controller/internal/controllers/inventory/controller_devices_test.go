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

package inventory

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestSummarizeDeviceStates(t *testing.T) {
	devices := []*v1alpha1.GPUDevice{
		{Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateReady}},
		{Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStatePendingAssignment}},
		{Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateAssigned}},
		{Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateReserved}},
		{Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateInUse}},
		{Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateFaulted}},
		{Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateValidating}},
		{Status: v1alpha1.GPUDeviceStatus{State: ""}}, // normalize -> Discovered -> Validating bucket
		{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{deviceIgnoreAnnotation: "true"},
			},
			Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateReady},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{deviceIgnoreLabel: "true"},
			},
			Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateReady},
		},
	}

	summary := summarizeDeviceStates(devices)
	if summary.Total != int32(len(devices)) {
		t.Fatalf("unexpected total %d", summary.Total)
	}
	if summary.Ready != 1 {
		t.Fatalf("expected 1 ready, got %d", summary.Ready)
	}
	if summary.PendingAssignment != 1 || summary.Assigned != 1 || summary.Reserved != 1 || summary.InUse != 1 {
		t.Fatalf("unexpected assigned counters: %+v", summary)
	}
	if summary.Validating != 2 {
		t.Fatalf("expected 2 validating, got %d", summary.Validating)
	}
	if summary.Faulted != 1 {
		t.Fatalf("expected 1 faulted, got %d", summary.Faulted)
	}
	if summary.Ignored != 2 {
		t.Fatalf("expected 2 ignored, got %d", summary.Ignored)
	}
}

func TestIsDeviceIgnored(t *testing.T) {
	if isDeviceIgnored(nil) {
		t.Fatal("nil device should not be ignored")
	}
	dev := &v1alpha1.GPUDevice{}
	if isDeviceIgnored(dev) {
		t.Fatal("unexpected ignore for empty device")
	}
	dev.Annotations = map[string]string{deviceIgnoreAnnotation: "true"}
	if !isDeviceIgnored(dev) {
		t.Fatal("expected ignore via annotation")
	}
	dev.Annotations = nil
	dev.Labels = map[string]string{deviceIgnoreLabel: "true"}
	if !isDeviceIgnored(dev) {
		t.Fatal("expected ignore via label")
	}
}
