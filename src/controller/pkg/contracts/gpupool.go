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

// PoolHandler reconciles GPUPool resources and produces device assignments.
type PoolHandler interface {
	Named
	HandlePool(ctx context.Context, pool *gpuv1alpha1.GPUPool) (Result, error)
}

// PoolRegistry stores pool handlers.
type PoolRegistry struct {
	*Registry[PoolHandler]
}

// NewPoolRegistry creates a new registry for pool handlers.
func NewPoolRegistry() *PoolRegistry {
	return &PoolRegistry{Registry: NewRegistry[PoolHandler]()}
}
