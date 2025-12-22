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

package state

import (
	"sort"
	"strings"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func parseMIGConfig(labels map[string]string) v1alpha1.GPUMIGConfig {
	cfg := v1alpha1.GPUMIGConfig{}

	if capableValue, ok := firstExisting(labels, gfdMigCapableLabel, gfdMigAltCapableLabel); ok {
		cfg.Capable = parseBool(capableValue)
	}

	if strategyValue, ok := firstExisting(labels, gfdMigStrategyLabel, gfdMigAltStrategy); ok {
		switch strings.ToLower(strategyValue) {
		case "single":
			cfg.Strategy = v1alpha1.GPUMIGStrategySingle
		case "mixed":
			cfg.Strategy = v1alpha1.GPUMIGStrategyMixed
		default:
			cfg.Strategy = v1alpha1.GPUMIGStrategyNone
		}
	}

	type migCountAccumulator struct {
		capability v1alpha1.GPUMIGTypeCapacity
		priority   int
	}

	typeAccumulator := map[string]*migCountAccumulator{}
	profiles := map[string]struct{}{}

	for key, value := range labels {
		if !strings.HasPrefix(key, migProfileLabelPrefix) {
			continue
		}

		trimmed := strings.TrimPrefix(key, migProfileLabelPrefix)
		firstDot := strings.Index(trimmed, ".")
		if firstDot == -1 {
			continue
		}
		secondDot := strings.Index(trimmed[firstDot+1:], ".")
		if secondDot == -1 {
			continue
		}
		secondDot += firstDot + 1

		profileCore := trimmed[:secondDot]
		metric := trimmed[secondDot+1:]
		if metric == "" {
			continue
		}

		profileName := strings.ToLower(profileCore)

		count := parseInt32(value)
		if count == 0 && value == "" {
			continue
		}

		profiles[profileName] = struct{}{}

		priority := 0
		switch metric {
		case "count":
			priority = 3
		case "ready":
			priority = 2
		case "available":
			priority = 1
		default:
			continue
		}

		entry := typeAccumulator[profileName]
		if entry == nil {
			entry = &migCountAccumulator{capability: v1alpha1.GPUMIGTypeCapacity{Name: profileName}}
			typeAccumulator[profileName] = entry
		}
		if priority > entry.priority {
			entry.priority = priority
			entry.capability.Count = count
		}

	}

	if len(profiles) > 0 {
		cfg.ProfilesSupported = make([]string, 0, len(profiles))
		for profile := range profiles {
			cfg.ProfilesSupported = append(cfg.ProfilesSupported, profile)
		}
		sort.Strings(cfg.ProfilesSupported)
	}

	if len(typeAccumulator) > 0 {
		cfg.Types = make([]v1alpha1.GPUMIGTypeCapacity, 0, len(typeAccumulator))
		for _, entry := range typeAccumulator {
			cfg.Types = append(cfg.Types, entry.capability)
		}
		sort.Slice(cfg.Types, func(i, j int) bool {
			return cfg.Types[i].Name < cfg.Types[j].Name
		})
	}

	return cfg
}

func migConfigEmpty(cfg v1alpha1.GPUMIGConfig) bool {
	return !cfg.Capable && cfg.Strategy == "" && len(cfg.ProfilesSupported) == 0 && len(cfg.Types) == 0
}
