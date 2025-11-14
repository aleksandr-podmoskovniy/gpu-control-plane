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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"
)

func TestNamesReturnsComponentsCopy(t *testing.T) {
	names := Names()
	if len(names) != len(definitions) {
		t.Fatalf("expected %d components, got %d", len(definitions), len(names))
	}
	names[0] = meta.Component("modified")
	if Names()[0] == meta.Component("modified") {
		t.Fatal("expected Names() to return a copy")
	}
}

func TestEnabledComponentsByPhase(t *testing.T) {
	testCases := []struct {
		phase    v1alpha1.GPUNodeBootstrapPhase
		expected []meta.Component
	}{
		{phase: v1alpha1.GPUNodeBootstrapPhaseDisabled, expected: nil},
		{phase: v1alpha1.GPUNodeBootstrapPhaseValidating, expected: []meta.Component{
			meta.ComponentValidator,
		}},
		{phase: v1alpha1.GPUNodeBootstrapPhaseGFD, expected: []meta.Component{
			meta.ComponentValidator,
			meta.ComponentGPUFeatureDiscovery,
		}},
		{phase: v1alpha1.GPUNodeBootstrapPhaseMonitoring, expected: []meta.Component{
			meta.ComponentValidator,
			meta.ComponentGPUFeatureDiscovery,
			meta.ComponentDCGM,
			meta.ComponentDCGMExporter,
		}},
		{phase: v1alpha1.GPUNodeBootstrapPhaseReady, expected: []meta.Component{
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
	set := EnabledComponents(v1alpha1.GPUNodeBootstrapPhaseReady, false)
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
