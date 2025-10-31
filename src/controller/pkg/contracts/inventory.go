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

package contracts

import (
	"context"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

// InventoryHandler reconciles a single GPUDevice object and may produce status updates or events.
type InventoryHandler interface {
	Named
	HandleDevice(ctx context.Context, device *gpuv1alpha1.GPUDevice) (Result, error)
}

// InventoryRegistry stores inventory handlers.
type InventoryRegistry struct {
	*Registry[InventoryHandler]
}

// NewInventoryRegistry returns an empty inventory registry.
func NewInventoryRegistry() *InventoryRegistry {
	return &InventoryRegistry{Registry: NewRegistry[InventoryHandler]()}
}
