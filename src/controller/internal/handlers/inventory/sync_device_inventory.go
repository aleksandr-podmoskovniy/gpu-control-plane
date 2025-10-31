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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// DeviceInventorySync copies device info into the owning node inventory status.
type DeviceInventorySync struct {
	log    logr.Logger
	client client.Client
}

func NewDeviceInventorySync(log logr.Logger, c client.Client) *DeviceInventorySync {
	return &DeviceInventorySync{log: log, client: c}
}

func (h *DeviceInventorySync) Name() string {
	return "device-inventory-sync"
}

func (h *DeviceInventorySync) HandleDevice(ctx context.Context, device *v1alpha1.GPUDevice) (contracts.Result, error) {
	if device.Status.NodeName == "" {
		return contracts.Result{}, nil
	}
	inv := &v1alpha1.GPUNodeInventory{}
	if err := h.client.Get(ctx, types.NamespacedName{Name: device.Status.NodeName}, inv); err != nil {
		if apierrors.IsNotFound(err) {
			h.log.V(2).Info("node inventory not found", "node", device.Status.NodeName)
			return contracts.Result{}, nil
		}
		return contracts.Result{}, err
	}

	replaced := false
	for i := range inv.Status.Hardware.Devices {
		if inv.Status.Hardware.Devices[i].InventoryID == device.Status.InventoryID {
			inv.Status.Hardware.Devices[i].State = device.Status.State
			inv.Status.Hardware.Devices[i].AutoAttach = device.Status.AutoAttach
			replaced = true
			break
		}
	}
	if !replaced {
		inv.Status.Hardware.Devices = append(inv.Status.Hardware.Devices, v1alpha1.GPUNodeDevice{
			InventoryID: device.Status.InventoryID,
			Product:     device.Status.Hardware.Product,
			State:       device.Status.State,
			AutoAttach:  device.Status.AutoAttach,
		})
	}

	if err := h.client.Status().Update(ctx, inv); err != nil {
		if apierrors.IsConflict(err) {
			h.log.V(1).Info("conflict updating inventory, retrying", "node", inv.Name)
			return contracts.Result{Requeue: true}, nil
		}
		return contracts.Result{}, err
	}

	return contracts.Result{}, nil
}
