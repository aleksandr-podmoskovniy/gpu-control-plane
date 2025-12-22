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

package compatibility

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

const (
	conditionSupported = "Supported"
)

// CompatibilityCheckHandler ensures provider/backend are supported by current implementation.
type CompatibilityCheckHandler struct{}

func NewCompatibilityCheckHandler() *CompatibilityCheckHandler {
	return &CompatibilityCheckHandler{}
}

func (h *CompatibilityCheckHandler) Name() string {
	return "compatibility-check"
}

func (h *CompatibilityCheckHandler) HandlePool(_ context.Context, pool *v1alpha1.GPUPool) (reconcile.Result, error) {
	supportedProvider := pool.Spec.Provider == "" || pool.Spec.Provider == "Nvidia"
	supportedBackend := pool.Spec.Backend == "" || pool.Spec.Backend == "DevicePlugin"

	cond := metav1.Condition{
		Type:               conditionSupported,
		Status:             metav1.ConditionTrue,
		Reason:             "Supported",
		Message:            "provider/backend supported",
		ObservedGeneration: pool.Generation,
	}

	if !supportedProvider {
		cond.Status = metav1.ConditionFalse
		cond.Reason = "UnsupportedProvider"
		cond.Message = "only provider=Nvidia is supported"
	}
	if supportedProvider && !supportedBackend {
		cond.Status = metav1.ConditionFalse
		cond.Reason = "UnsupportedBackend"
		cond.Message = "only backend=DevicePlugin is supported"
	}

	conds := pool.Status.Conditions
	meta.SetStatusCondition(&conds, cond)
	pool.Status.Conditions = conds

	return reconcile.Result{}, nil
}
