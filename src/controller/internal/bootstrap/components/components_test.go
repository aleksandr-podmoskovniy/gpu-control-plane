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
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"
)

func TestEnabledComponentsByPhase(t *testing.T) {
	testCases := []struct {
		phase    Phase
		expected []meta.Component
	}{
		{phase: PhaseDisabled, expected: nil},
		{phase: PhaseValidating, expected: []meta.Component{
			meta.ComponentValidator,
		}},
		{phase: PhaseGFD, expected: []meta.Component{
			meta.ComponentValidator,
		}},
		{phase: PhaseMonitoring, expected: []meta.Component{
			meta.ComponentValidator,
			meta.ComponentGPUFeatureDiscovery,
			meta.ComponentDCGM,
			meta.ComponentDCGMExporter,
		}},
		{phase: PhaseReady, expected: []meta.Component{
			meta.ComponentValidator,
			meta.ComponentGPUFeatureDiscovery,
			meta.ComponentDCGM,
			meta.ComponentDCGMExporter,
		}},
	}

	for _, tc := range testCases {
		t.Run(string(tc.phase), func(t *testing.T) {
			set := EnabledComponents(tc.phase, true)
			if len(set) != len(tc.expected) {
				t.Fatalf("expected %d components, got %d (%v)", len(tc.expected), len(set), set)
			}
			for _, component := range tc.expected {
				if !set[component] {
					t.Fatalf("component %s missing in phase %s", component, tc.phase)
				}
			}
		})
	}
}

func TestEnabledComponentsDefaultsToValidating(t *testing.T) {
	set := EnabledComponents("unknown", true)
	if len(set) != 1 || !set[meta.ComponentValidator] {
		t.Fatalf("expected validator enabled for unknown phase, got %v", set)
	}
}

func TestEnabledComponentsDisablesGPUWorkloadsWhenNoDevices(t *testing.T) {
	set := EnabledComponents(PhaseReady, false)
	for _, component := range []meta.Component{
		meta.ComponentGPUFeatureDiscovery,
		meta.ComponentDCGM,
		meta.ComponentDCGMExporter,
	} {
		if set[component] {
			t.Fatalf("component %s expected to be disabled when no devices present", component)
		}
	}
	if !set[meta.ComponentValidator] {
		t.Fatalf("validator should stay enabled in Ready phase")
	}
}
