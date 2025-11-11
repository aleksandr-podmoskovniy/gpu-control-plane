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

package components

import (
	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"
)

// Definition describes a bootstrap workload and the phase when it must be enabled.
type Definition struct {
	Name           meta.Component
	ActivationFrom v1alpha1.GPUNodeBootstrapPhase
}

var definitions = []Definition{
	{Name: meta.ComponentValidator, ActivationFrom: v1alpha1.GPUNodeBootstrapPhaseValidating},
	{Name: meta.ComponentGPUFeatureDiscovery, ActivationFrom: v1alpha1.GPUNodeBootstrapPhaseGFD},
	{Name: meta.ComponentDCGM, ActivationFrom: v1alpha1.GPUNodeBootstrapPhaseMonitoring},
	{Name: meta.ComponentDCGMExporter, ActivationFrom: v1alpha1.GPUNodeBootstrapPhaseMonitoring},
}

var phaseRank = map[v1alpha1.GPUNodeBootstrapPhase]int{
	v1alpha1.GPUNodeBootstrapPhaseDisabled:         0,
	v1alpha1.GPUNodeBootstrapPhaseValidating:       1,
	v1alpha1.GPUNodeBootstrapPhaseValidatingFailed: 1,
	v1alpha1.GPUNodeBootstrapPhaseGFD:              2,
	v1alpha1.GPUNodeBootstrapPhaseMonitoring:       3,
	v1alpha1.GPUNodeBootstrapPhaseReady:            4,
}

// EnabledComponents returns the map of components that must be active for the phase.
func EnabledComponents(phase v1alpha1.GPUNodeBootstrapPhase) map[meta.Component]bool {
	rank, ok := phaseRank[phase]
	if !ok {
		rank = phaseRank[v1alpha1.GPUNodeBootstrapPhaseValidating]
	}

	result := make(map[meta.Component]bool, len(definitions))
	if rank == 0 {
		return result
	}

	for _, def := range definitions {
		if rank >= phaseRank[def.ActivationFrom] {
			result[def.Name] = true
		}
	}
	return result
}

// Names returns the list of component identifiers.
func Names() []meta.Component {
	names := make([]meta.Component, len(definitions))
	for i, def := range definitions {
		names[i] = def.Name
	}
	return names
}
