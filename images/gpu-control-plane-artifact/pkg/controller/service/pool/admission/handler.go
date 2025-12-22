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
	"strings"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/admission/validators"
)

// PoolValidationHandler validates backend/provider/slicing constraints and applies safe defaults.
type PoolValidationHandler struct {
	log logr.Logger
}

func NewPoolValidationHandler(log logr.Logger) *PoolValidationHandler {
	return &PoolValidationHandler{log: log.WithName("pool-validation")}
}

func (h *PoolValidationHandler) Name() string {
	return "pool-validation"
}

func (h *PoolValidationHandler) SyncPool(_ context.Context, pool *v1alpha1.GPUPool) (reconcile.Result, error) {
	if strings.TrimSpace(pool.Name) == "" {
		return reconcile.Result{}, fmt.Errorf("metadata.name must be set")
	}
	applyDefaults(&pool.Spec)

	checks := []validators.SpecValidator{
		validators.Provider(defaultProvider),
		validators.Resource(),
		validators.Selectors(),
		validators.Scheduling(),
	}
	if err := validators.Run(checks, &pool.Spec); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
