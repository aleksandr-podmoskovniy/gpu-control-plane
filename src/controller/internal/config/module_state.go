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

package config

import (
	"encoding/json"

	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
)

// ModuleSettingsToState converts static controller configuration into ModuleConfig state used by runtime store.
func ModuleSettingsToState(settings ModuleSettings) (moduleconfig.State, error) {
	input := moduleconfig.Input{
		Settings: map[string]any{
			"managedNodes": map[string]any{
				"labelKey":         settings.ManagedNodes.LabelKey,
				"enabledByDefault": settings.ManagedNodes.EnabledByDefault,
			},
			"deviceApproval": map[string]any{
				"mode": string(settings.DeviceApproval.Mode),
			},
			"scheduling": map[string]any{
				"defaultStrategy": settings.Scheduling.DefaultStrategy,
				"topologyKey":     settings.Scheduling.TopologyKey,
			},
		},
	}

	if settings.DeviceApproval.Selector != nil {
		if data, err := json.Marshal(settings.DeviceApproval.Selector); err == nil {
			var selector map[string]any
			if err := json.Unmarshal(data, &selector); err == nil {
				input.Settings["deviceApproval"].(map[string]any)["selector"] = selector
			}
		}
	}

	return moduleconfig.Parse(input)
}
