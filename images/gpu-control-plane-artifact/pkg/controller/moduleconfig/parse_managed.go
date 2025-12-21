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
	"strings"
)

func parseManaged(raw json.RawMessage) (ManagedNodesSettings, error) {
	settings := ManagedNodesSettings{LabelKey: DefaultNodeLabelKey, EnabledByDefault: true}
	if len(raw) == 0 || string(raw) == "null" {
		return settings, nil
	}
	var payload struct {
		LabelKey         string `json:"labelKey"`
		EnabledByDefault *bool  `json:"enabledByDefault"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return settings, fmt.Errorf("decode managedNodes: %w", err)
	}
	if v := strings.TrimSpace(payload.LabelKey); v != "" {
		settings.LabelKey = v
	}
	if payload.EnabledByDefault != nil {
		settings.EnabledByDefault = *payload.EnabledByDefault
	}
	return settings, nil
}
