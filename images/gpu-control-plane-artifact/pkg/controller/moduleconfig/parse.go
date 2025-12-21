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

package moduleconfig

import (
	"encoding/json"
	"fmt"
)

func Parse(input Input) (State, error) {
	state := DefaultState()
	if input.Enabled != nil {
		state.Enabled = *input.Enabled
	}

	raw := make(map[string]json.RawMessage, len(input.Settings))
	for key, value := range input.Settings {
		data, err := json.Marshal(value)
		if err != nil {
			return state, fmt.Errorf("encode settings.%s: %w", key, err)
		}
		raw[key] = data
	}

	managed, err := parseManaged(raw["managedNodes"])
	if err != nil {
		return state, err
	}
	state.Settings.ManagedNodes = managed
	state.Sanitized["managedNodes"] = map[string]any{"labelKey": managed.LabelKey, "enabledByDefault": managed.EnabledByDefault}

	approval, selector, err := parseApproval(raw["deviceApproval"])
	if err != nil {
		return state, err
	}
	state.Settings.DeviceApproval = approval
	m := map[string]any{"mode": string(approval.Mode)}
	if selector != nil {
		m["selector"] = selector
	}
	state.Sanitized["deviceApproval"] = m

	scheduling, err := parseScheduling(raw["scheduling"])
	if err != nil {
		return state, err
	}
	state.Settings.Scheduling = scheduling
	state.Sanitized["scheduling"] = map[string]any{"defaultStrategy": scheduling.DefaultStrategy, "topologyKey": scheduling.TopologyKey}

	monitoring, err := parseMonitoring(raw["monitoring"])
	if err != nil {
		return state, err
	}
	state.Settings.Monitoring = monitoring
	state.Sanitized["monitoring"] = map[string]any{"serviceMonitor": monitoring.ServiceMonitor}

	placement := parsePlacement(raw["placement"])
	state.Settings.Placement = placement
	state.Sanitized["placement"] = map[string]any{"customTolerationKeys": placement.CustomTolerationKeys}

	inventory, err := parseInventory(raw["inventory"])
	if err != nil {
		return state, err
	}
	state.Inventory = inventory
	state.Sanitized["inventory"] = map[string]any{"resyncPeriod": inventory.ResyncPeriod}

	https, httpsMap, err := parseHTTPS(raw["https"], input.Global)
	if err != nil {
		return state, err
	}
	state.HTTPS = https
	state.Sanitized["https"] = httpsMap

	if ha := parseBool(raw["highAvailability"]); ha != nil {
		state.HighAvailability = ha
		state.Sanitized["highAvailability"] = *ha
	}

	return state, nil
}
