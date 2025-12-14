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

import "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"

type Phase string

const (
	PhaseDisabled         Phase = "Disabled"
	PhaseValidating       Phase = "Validating"
	PhaseValidatingFailed Phase = "ValidatingFailed"
	PhaseGFD              Phase = "GFD"
	PhaseMonitoring       Phase = "Monitoring"
	PhaseReady            Phase = "Ready"
)

// Definition describes a bootstrap workload and the phase when it must be enabled.
type Definition struct {
	Name           meta.Component
	ActivationFrom Phase
}

var definitions = []Definition{
	{Name: meta.ComponentValidator, ActivationFrom: PhaseValidating},
	{Name: meta.ComponentGPUFeatureDiscovery, ActivationFrom: PhaseMonitoring},
	{Name: meta.ComponentDCGM, ActivationFrom: PhaseMonitoring},
	{Name: meta.ComponentDCGMExporter, ActivationFrom: PhaseMonitoring},
}

var phaseRank = map[Phase]int{
	PhaseDisabled:         0,
	PhaseValidating:       1,
	PhaseValidatingFailed: 1,
	PhaseGFD:              2,
	PhaseMonitoring:       3,
	PhaseReady:            4,
}

// EnabledComponents returns the map of components that must be active for the phase.
// Only validator is enabled during Validating/ValidatingFailed; GPU workloads appear
// starting from their activation phase and require that hardware is present.
func EnabledComponents(
	phase Phase,
	devicesPresent bool,
) map[meta.Component]bool {
	rank, ok := phaseRank[phase]
	if !ok {
		rank = phaseRank[PhaseValidating]
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

	if !devicesPresent {
		delete(result, meta.ComponentGPUFeatureDiscovery)
		delete(result, meta.ComponentDCGM)
		delete(result, meta.ComponentDCGMExporter)
	}

	return result
}
