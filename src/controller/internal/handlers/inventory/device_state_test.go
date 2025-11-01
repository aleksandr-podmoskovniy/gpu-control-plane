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
	"context"
	"testing"

	"github.com/go-logr/logr/testr"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestDeviceStateHandlerSetsDefault(t *testing.T) {
	h := NewDeviceStateHandler(testr.New(t))
	device := &gpuv1alpha1.GPUDevice{}

	if _, err := h.HandleDevice(context.Background(), device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if device.Status.State != gpuv1alpha1.GPUDeviceStateUnassigned {
		t.Fatalf("state not defaulted: %s", device.Status.State)
	}
}

func TestDeviceStateHandlerName(t *testing.T) {
	h := NewDeviceStateHandler(testr.New(t))
	if h.Name() != "device-state" {
		t.Fatalf("unexpected handler name: %s", h.Name())
	}
}
