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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	invservice "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/service"
	corev1 "k8s.io/api/core/v1"
)

type failingDetectionsCollector struct {
	err error
}

func (f failingDetectionsCollector) Collect(context.Context, string) (invservice.NodeDetection, error) {
	return invservice.NodeDetection{}, f.err
}

type failingDeviceService struct {
	err error
}

func (f failingDeviceService) Reconcile(context.Context, *corev1.Node, deviceSnapshot, map[string]string, bool, DeviceApprovalPolicy, func(*v1alpha1.GPUDevice, deviceSnapshot)) (*v1alpha1.GPUDevice, contracts.Result, error) {
	return nil, contracts.Result{}, f.err
}

type fixedDeviceService struct {
	device *v1alpha1.GPUDevice
	result contracts.Result
}

func (f fixedDeviceService) Reconcile(context.Context, *corev1.Node, deviceSnapshot, map[string]string, bool, DeviceApprovalPolicy, func(*v1alpha1.GPUDevice, deviceSnapshot)) (*v1alpha1.GPUDevice, contracts.Result, error) {
	return f.device, f.result, nil
}

type failingCleanupService struct {
	err error
}

func (f failingCleanupService) CleanupNode(context.Context, string) error { return nil }

func (f failingCleanupService) DeleteInventory(context.Context, string) error { return nil }

func (f failingCleanupService) ClearMetrics(string) {}

func (f failingCleanupService) RemoveOrphans(context.Context, *corev1.Node, map[string]struct{}) error {
	return f.err
}

type noOpInventoryService struct{}

func (noOpInventoryService) Reconcile(context.Context, *corev1.Node, nodeSnapshot, []*v1alpha1.GPUDevice) error {
	return nil
}

func (noOpInventoryService) UpdateDeviceMetrics(string, []*v1alpha1.GPUDevice) {}
