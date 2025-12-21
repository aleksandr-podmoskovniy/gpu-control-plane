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
	"strings"
)

func parsePlacement(raw json.RawMessage) PlacementSettings {
	settings := PlacementSettings{CustomTolerationKeys: []string{}}
	if len(raw) == 0 || string(raw) == "null" {
		return settings
	}
	var payload struct {
		CustomTolerationKeys []string `json:"customTolerationKeys"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return settings
	}
	seen := map[string]struct{}{}
	keys := make([]string, 0, len(payload.CustomTolerationKeys))
	for _, k := range payload.CustomTolerationKeys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	settings.CustomTolerationKeys = keys
	return settings
}
