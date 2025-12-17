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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	cpmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics"
)

// DetectionCollector fetches detections from gfd-extender for a node.
type DetectionCollector interface {
	Collect(ctx context.Context, node string) (nodeDetection, error)
}

// CleanupService owns removal of inventory objects and related metrics.
type CleanupService interface {
	CleanupNode(ctx context.Context, nodeName string) error
	DeleteInventory(ctx context.Context, nodeName string) error
	ClearMetrics(nodeName string)
	RemoveOrphans(ctx context.Context, node *corev1.Node, orphanDevices map[string]struct{}) error
}

type detectionCollector struct {
	client client.Client
}

type cleanupService struct {
	client   client.Client
	recorder eventRecorder
}

func newDetectionCollector(c client.Client) DetectionCollector {
	return &detectionCollector{client: c}
}

func newCleanupService(c client.Client, recorder eventRecorder) CleanupService {
	return &cleanupService{client: c, recorder: recorder}
}

func (c *cleanupService) DeleteInventory(ctx context.Context, nodeName string) error {
	inventory := &v1alpha1.GPUNodeState{}
	if err := c.client.Get(ctx, client.ObjectKey{Name: nodeName}, inventory); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := c.client.Delete(ctx, inventory); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (c *cleanupService) ClearMetrics(nodeName string) {
	cpmetrics.InventoryDevicesDelete(nodeName)
	cpmetrics.InventoryConditionDelete(nodeName, conditionInventoryComplete)
	for _, state := range knownDeviceStates {
		cpmetrics.InventoryDeviceStateDelete(nodeName, string(state))
	}
}

func (c *cleanupService) CleanupNode(ctx context.Context, nodeName string) error {
	deviceList := &v1alpha1.GPUDeviceList{}
	if err := c.client.List(ctx, deviceList, client.MatchingFields{deviceNodeIndexKey: nodeName}); err != nil {
		return err
	}
	for i := range deviceList.Items {
		device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: deviceList.Items[i].Name}}
		if err := c.client.Delete(ctx, device); err != nil && !apierrors.IsNotFound(err) {
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
		if err := c.client.Delete(ctx, &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: name}}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if c.recorder != nil {
			c.recorder.Eventf(node, corev1.EventTypeNormal, eventDeviceRemoved, "GPU device %s removed from inventory", name)
		}
	}
	return nil
}
