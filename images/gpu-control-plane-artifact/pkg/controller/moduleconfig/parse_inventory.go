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
	"regexp"
	"strings"
	"time"
)

var inventoryResyncPattern = regexp.MustCompile(`^\d+(s|m|h)$`)

func parseInventory(raw json.RawMessage) (InventorySettings, error) {
	settings := InventorySettings{ResyncPeriod: DefaultInventoryResyncPeriod}
	if len(raw) == 0 || string(raw) == "null" {
		return settings, nil
	}
	var payload struct {
		ResyncPeriod string `json:"resyncPeriod"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return settings, fmt.Errorf("decode inventory settings: %w", err)
	}
	if trimmed := strings.TrimSpace(payload.ResyncPeriod); trimmed != "" {
		if !inventoryResyncPattern.MatchString(trimmed) {
			return settings, fmt.Errorf("parse inventory.resyncPeriod: value %q does not match ^\\d+(s|m|h)$", trimmed)
		}
		if _, err := time.ParseDuration(trimmed); err != nil {
			return settings, fmt.Errorf("parse inventory.resyncPeriod: %w", err)
		}
		settings.ResyncPeriod = trimmed
	}
	return settings, nil
}
