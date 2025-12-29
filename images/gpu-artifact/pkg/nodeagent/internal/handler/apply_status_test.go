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

package handler

import (
	"testing"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

func TestDriverTypeFromName(t *testing.T) {
	tests := []struct {
		name  string
		want  gpuv1alpha1.DriverType
		input string
	}{
		{name: "nvidia", input: "nvidia", want: gpuv1alpha1.DriverTypeNvidia},
		{name: "vfio", input: "vfio-pci", want: gpuv1alpha1.DriverTypeVFIO},
		{name: "rocm", input: "amdgpu", want: gpuv1alpha1.DriverTypeROCm},
		{name: "unknown", input: "unknown", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := driverTypeFromName(tt.input); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestBuildStatusNilNodeInfo(t *testing.T) {
	obj := &gpuv1alpha1.PhysicalGPU{}
	dev := state.Device{Address: "0000:01:00.0"}

	status := buildStatus(obj, dev, "node-1", nil)
	if status.NodeInfo == nil || status.NodeInfo.NodeName != "node-1" {
		t.Fatalf("expected nodeInfo.nodeName node-1, got %#v", status.NodeInfo)
	}
	if status.CurrentState != nil {
		t.Fatalf("expected currentState nil when no driver, got %#v", status.CurrentState)
	}
}

func TestBuildStatusKeepsDriverType(t *testing.T) {
	obj := &gpuv1alpha1.PhysicalGPU{
		Status: gpuv1alpha1.PhysicalGPUStatus{
			CurrentState: &gpuv1alpha1.GPUCurrentState{
				DriverType: gpuv1alpha1.DriverTypeVFIO,
			},
		},
	}
	dev := state.Device{DriverName: ""}

	status := buildStatus(obj, dev, "node-1", &gpuv1alpha1.NodeInfo{})
	if status.CurrentState == nil || status.CurrentState.DriverType != gpuv1alpha1.DriverTypeVFIO {
		t.Fatalf("expected driverType VFIO, got %#v", status.CurrentState)
	}
}

func TestBuildStatusOverridesDriverType(t *testing.T) {
	obj := &gpuv1alpha1.PhysicalGPU{
		Status: gpuv1alpha1.PhysicalGPUStatus{
			CurrentState: &gpuv1alpha1.GPUCurrentState{
				DriverType: gpuv1alpha1.DriverTypeVFIO,
			},
		},
	}
	dev := state.Device{DriverName: "nvidia"}

	status := buildStatus(obj, dev, "node-1", &gpuv1alpha1.NodeInfo{})
	if status.CurrentState == nil || status.CurrentState.DriverType != gpuv1alpha1.DriverTypeNvidia {
		t.Fatalf("expected driverType Nvidia, got %#v", status.CurrentState)
	}
}
