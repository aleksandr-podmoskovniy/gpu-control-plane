/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package health

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
)

const filterHealthyHandlerName = "filter-healthy"

// FilterHealthyHandler selects PhysicalGPU objects that are HardwareHealthy.
type FilterHealthyHandler struct{}

// NewFilterHealthyHandler constructs a filter handler.
func NewFilterHealthyHandler() *FilterHealthyHandler {
	return &FilterHealthyHandler{}
}

// Name returns the handler name.
func (h *FilterHealthyHandler) Name() string {
	return filterHealthyHandlerName
}

// Handle filters HardwareHealthy GPUs into the ready list.
func (h *FilterHealthyHandler) Handle(_ context.Context, st state.State) error {
	ready := make([]gpuv1alpha1.PhysicalGPU, 0, len(st.Ready()))
	for _, pgpu := range st.Ready() {
		cond := meta.FindStatusCondition(pgpu.Status.Conditions, handler.HardwareHealthyType)
		if cond != nil && cond.Status == metav1.ConditionTrue {
			ready = append(ready, pgpu)
		}
	}
	st.SetReady(ready)
	return nil
}
