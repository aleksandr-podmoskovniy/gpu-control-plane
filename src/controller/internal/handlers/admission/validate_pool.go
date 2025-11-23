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

package admission

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

const (
	defaultProvider = "nvidia"
	defaultBackend  = "device-plugin"
)

// PoolValidationHandler validates backend/provider/slicing constraints and applies safe defaults.
type PoolValidationHandler struct {
	log logr.Logger
}

func NewPoolValidationHandler(log logr.Logger) *PoolValidationHandler {
	return &PoolValidationHandler{log: log}
}

func (h *PoolValidationHandler) Name() string {
	return "pool-validation"
}

func (h *PoolValidationHandler) SyncPool(_ context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	if pool.Spec.Provider == "" {
		pool.Spec.Provider = defaultProvider
	}
	if pool.Spec.Backend == "" {
		pool.Spec.Backend = defaultBackend
	}
	if pool.Spec.Allocation.SlicesPerUnit == 0 {
		pool.Spec.Allocation.SlicesPerUnit = 1
	}

	if err := h.validateProvider(pool.Spec.Provider); err != nil {
		return contracts.Result{}, err
	}
	if err := h.validateAllocation(&pool.Spec); err != nil {
		return contracts.Result{}, err
	}
	return contracts.Result{}, nil
}

func (h *PoolValidationHandler) validateProvider(provider string) error {
	if provider != defaultProvider {
		return fmt.Errorf("unsupported provider %q", provider)
	}
	return nil
}

func (h *PoolValidationHandler) validateAllocation(spec *v1alpha1.GPUPoolSpec) error {
	if spec.Allocation.Mode == "" {
		return fmt.Errorf("allocation.mode must be set")
	}
	if spec.Allocation.SlicesPerUnit < 1 {
		return fmt.Errorf("allocation.slicesPerUnit must be >= 1")
	}
	switch spec.Allocation.Mode {
	case v1alpha1.GPUPoolAllocationCard:
		if spec.Allocation.MIGProfile != "" {
			return fmt.Errorf("migProfile is not allowed when mode=Card")
		}
	case v1alpha1.GPUPoolAllocationMIG:
		if spec.Allocation.MIGProfile == "" {
			return fmt.Errorf("migProfile is required when mode=MIG")
		}
	default:
		return fmt.Errorf("unsupported allocation mode %q", spec.Allocation.Mode)
	}

	if spec.Backend == "dra" {
		if spec.Allocation.Mode != v1alpha1.GPUPoolAllocationCard {
			return fmt.Errorf("backend=dra currently supports only mode=Card")
		}
		if spec.Allocation.SlicesPerUnit > 1 {
			return fmt.Errorf("backend=dra does not support slicesPerUnit>1")
		}
	}
	return nil
}
