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
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestInventoryServiceUpdateDeviceMetrics(t *testing.T) {
	svc := &InventoryService{}
	nodeName := "node-metrics"
	svc.UpdateDeviceMetrics(nodeName, []*v1alpha1.GPUDevice{
		{Status: v1alpha1.GPUDeviceStatus{State: ""}},
		{Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateReady}},
	})
}
