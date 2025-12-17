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

package pool

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
)

const conditionConfigured = "Configured"

// ConfigCheckHandler validates binding/local semantics against cluster pools and reports condition.
type ConfigCheckHandler struct {
	client client.Client
}

func NewConfigCheckHandler(c client.Client) *ConfigCheckHandler {
	return &ConfigCheckHandler{client: c}
}

func (h *ConfigCheckHandler) Name() string {
	return "config-check"
}

func (h *ConfigCheckHandler) HandlePool(ctx context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	if h.client == nil {
		return contracts.Result{}, nil
	}

	cond := metav1.Condition{
		Type:               conditionConfigured,
		Status:             metav1.ConditionTrue,
		Reason:             "Configured",
		Message:            "pool configuration is valid",
		ObservedGeneration: pool.Generation,
	}

	if pool.Namespace != "" {
		// Namespaced pool: ensure no ClusterGPUPool with the same name exists.
		cluster := &v1alpha1.ClusterGPUPool{}
		if err := h.client.Get(ctx, client.ObjectKey{Name: pool.Name}, cluster); err == nil {
			cond.Status = metav1.ConditionFalse
			cond.Reason = "NameCollision"
			cond.Message = "ClusterGPUPool with the same name exists"
		} else if !errors.IsNotFound(err) {
			return contracts.Result{}, err
		}
	}
	meta.SetStatusCondition(&pool.Status.Conditions, cond)
	if cond.Status == metav1.ConditionFalse {
		return contracts.Result{}, reconciler.ErrStopChain
	}
	return contracts.Result{}, nil
}
