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

	"github.com/go-logr/logr"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// DeviceStateHandler inspects a GPUDevice and normalises its status fields.
type DeviceStateHandler struct {
	log logr.Logger
}

func NewDeviceStateHandler(log logr.Logger) *DeviceStateHandler {
	return &DeviceStateHandler{log: log}
}

func (h *DeviceStateHandler) Name() string {
	return "device-state"
}

func (h *DeviceStateHandler) HandleDevice(_ context.Context, device *v1alpha1.GPUDevice) (contracts.Result, error) {
	switch device.Status.State {
	case "", v1alpha1.GPUDeviceStateUnassigned:
		h.log.V(2).Info("normalising device state to Discovered", "device", device.Name)
		device.Status.State = v1alpha1.GPUDeviceStateDiscovered
	}
	return contracts.Result{}, nil
}
