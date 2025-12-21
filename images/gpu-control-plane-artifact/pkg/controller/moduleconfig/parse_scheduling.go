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

func parseScheduling(raw json.RawMessage) (SchedulingSettings, error) {
	settings := SchedulingSettings{DefaultStrategy: DefaultSchedulingStrategy, TopologyKey: DefaultSchedulingTopology}
	if len(raw) == 0 || string(raw) == "null" {
		return settings, nil
	}
	var payload struct {
		DefaultStrategy string `json:"defaultStrategy"`
		TopologyKey     string `json:"topologyKey"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return settings, fmt.Errorf("decode scheduling settings: %w", err)
	}
	if strat := normalizeStrategy(payload.DefaultStrategy); strat != "" {
		settings.DefaultStrategy = strat
	} else if strings.TrimSpace(payload.DefaultStrategy) != "" {
		return settings, fmt.Errorf("unknown scheduling.defaultStrategy %q", payload.DefaultStrategy)
	}
	topo := strings.TrimSpace(payload.TopologyKey)
	if settings.DefaultStrategy == DefaultSchedulingStrategy && topo == "" {
		topo = DefaultSchedulingTopology
	}
	settings.TopologyKey = topo
	return settings, nil
}

func normalizeStrategy(strategy string) string {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "":
		return ""
	case "spread":
		return "Spread"
	case "binpack":
		return "BinPack"
	default:
		return ""
	}
}
