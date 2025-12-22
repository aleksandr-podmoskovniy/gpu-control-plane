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

package handler

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestDeviceStateHandlerDefaultsToDiscovered(t *testing.T) {
	h := NewDeviceStateHandler(testr.New(t))
	dev := &v1alpha1.GPUDevice{}

	_, err := h.HandleDevice(context.Background(), dev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev.Status.State != v1alpha1.GPUDeviceStateDiscovered {
		t.Fatalf("expected state to be Discovered, got %q", dev.Status.State)
	}
}

func TestDeviceStateHandlerPreservesExistingState(t *testing.T) {
	h := NewDeviceStateHandler(testr.New(t))
	dev := &v1alpha1.GPUDevice{}
	dev.Status.State = v1alpha1.GPUDeviceStateReady

	_, err := h.HandleDevice(context.Background(), dev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev.Status.State != v1alpha1.GPUDeviceStateReady {
		t.Fatalf("expected state to stay Ready, got %q", dev.Status.State)
	}
}

func TestDeviceStateHandlerName(t *testing.T) {
	h := NewDeviceStateHandler(testr.New(t))
	if h.Name() != "device-state" {
		t.Fatalf("unexpected handler name: %q", h.Name())
	}
}
