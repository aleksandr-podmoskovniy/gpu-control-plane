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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invservice "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/service"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
)

type DeviceService interface {
	Reconcile(
		ctx context.Context,
		node *corev1.Node,
		snapshot invstate.DeviceSnapshot,
		nodeLabels map[string]string,
		managed bool,
		approval invstate.DeviceApprovalPolicy,
		applyDetection func(*v1alpha1.GPUDevice, invstate.DeviceSnapshot),
	) (*v1alpha1.GPUDevice, reconcile.Result, error)
}

type InventoryService interface {
	Reconcile(ctx context.Context, node *corev1.Node, snapshot invstate.NodeSnapshot, devices []*v1alpha1.GPUDevice) error
	UpdateDeviceMetrics(nodeName string, devices []*v1alpha1.GPUDevice)
}

type DetectionCollector = invservice.DetectionCollector
type CleanupService = invservice.CleanupService
