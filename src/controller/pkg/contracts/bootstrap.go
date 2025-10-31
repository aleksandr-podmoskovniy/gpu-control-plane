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

// BootstrapHandler reconciles GPUNodeInventory resources, preparing nodes for pooling.
type BootstrapHandler interface {
	Named
	HandleNode(ctx context.Context, inventory *gpuv1alpha1.GPUNodeInventory) (Result, error)
}

// BootstrapRegistry keeps bootstrap handlers.
type BootstrapRegistry struct {
	*Registry[BootstrapHandler]
}

// NewBootstrapRegistry creates an empty registry for bootstrap handlers.
func NewBootstrapRegistry() *BootstrapRegistry {
	return &BootstrapRegistry{Registry: NewRegistry[BootstrapHandler]()}
}
