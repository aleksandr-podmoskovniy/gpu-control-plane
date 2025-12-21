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

package service

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
	invconsts "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/consts"
	invmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics/inventory"
)

// CleanupService owns removal of inventory objects and related metrics.
type CleanupService interface {
	CleanupNode(ctx context.Context, nodeName string) error
	DeleteInventory(ctx context.Context, nodeName string) error
	ClearMetrics(nodeName string)
	RemoveOrphans(ctx context.Context, node *corev1.Node, orphanDevices map[string]struct{}) error
}

type cleanupService struct {
	client   client.Client
	recorder record.EventRecorder
}

func NewCleanupService(c client.Client, recorder record.EventRecorder) CleanupService {
	return &cleanupService{client: c, recorder: recorder}
}

func (c *cleanupService) DeleteInventory(ctx context.Context, nodeName string) error {
	inventory, err := commonobject.FetchObject(ctx, client.ObjectKey{Name: nodeName}, c.client, &v1alpha1.GPUNodeState{})
	if err != nil {
		return err
	}
	return commonobject.DeleteObject(ctx, c.client, inventory)
}

func (c *cleanupService) ClearMetrics(nodeName string) {
	invmetrics.InventoryDevicesDelete(nodeName)
	invmetrics.InventoryConditionDelete(nodeName, invconsts.ConditionInventoryComplete)
	for _, state := range knownDeviceStates {
		invmetrics.InventoryDeviceStateDelete(nodeName, string(state))
	}
}

func (c *cleanupService) CleanupNode(ctx context.Context, nodeName string) error {
	deviceList := &v1alpha1.GPUDeviceList{}
	if err := c.client.List(ctx, deviceList, client.MatchingFields{invconsts.DeviceNodeIndexKey: nodeName}); err != nil {
		return err
	}
	for i := range deviceList.Items {
		device := &v1alpha1.GPUDevice{}
		device.Name = deviceList.Items[i].Name
		if err := commonobject.DeleteObject(ctx, c.client, device); err != nil {
			return err
		}
	}

	if err := c.DeleteInventory(ctx, nodeName); err != nil {
		return err
	}
	c.ClearMetrics(nodeName)

	return nil
}

func (c *cleanupService) RemoveOrphans(ctx context.Context, node *corev1.Node, orphanDevices map[string]struct{}) error {
	if len(orphanDevices) == 0 {
		return nil
	}
	for name := range orphanDevices {
		device := &v1alpha1.GPUDevice{}
		device.Name = name
		if err := commonobject.DeleteObject(ctx, c.client, device); err != nil {
			return err
		}
		if c.recorder != nil {
			c.recorder.Eventf(node, corev1.EventTypeNormal, invconsts.EventDeviceRemoved, "GPU device %s removed from inventory", name)
		}
	}
	return nil
}
