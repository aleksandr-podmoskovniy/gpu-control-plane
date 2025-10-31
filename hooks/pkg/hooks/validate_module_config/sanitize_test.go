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

package validate_module_config

import (
	"reflect"
	"testing"
)

func TestSanitizeModuleSettingsDefaults(t *testing.T) {
	cfg, err := sanitizeModuleSettings(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	managed, ok := cfg["managedNodes"].(map[string]any)
	if !ok {
		t.Fatalf("managedNodes not present in sanitized config: %#v", cfg)
	}
	if managed["labelKey"] != "gpu.deckhouse.io/enabled" {
		t.Fatalf("unexpected default labelKey: %v", managed["labelKey"])
	}
	if managed["enabledByDefault"] != true {
		t.Fatalf("expected enabledByDefault to be true")
	}

	approval, ok := cfg["deviceApproval"].(map[string]any)
	if !ok {
		t.Fatalf("deviceApproval not present in sanitized config: %#v", cfg)
	}
	if approval["mode"] != "Manual" {
		t.Fatalf("unexpected default deviceApproval.mode: %v", approval["mode"])
	}
	if _, exists := approval["selector"]; exists {
		t.Fatalf("selector must not be present for manual mode: %#v", approval)
	}

	scheduling, ok := cfg["scheduling"].(map[string]any)
	if !ok {
		t.Fatalf("scheduling not present in sanitized config: %#v", cfg)
	}
	if scheduling["defaultStrategy"] != "Spread" {
		t.Fatalf("unexpected default scheduling.defaultStrategy: %v", scheduling["defaultStrategy"])
	}
	if scheduling["topologyKey"] != "topology.kubernetes.io/zone" {
		t.Fatalf("unexpected default scheduling.topologyKey: %v", scheduling["topologyKey"])
	}

	inventory, ok := cfg["inventory"].(map[string]any)
	if !ok {
		t.Fatalf("inventory not present in sanitized config: %#v", cfg)
	}
	if inventory["resyncPeriod"] != "30s" {
		t.Fatalf("unexpected default inventory.resyncPeriod: %v", inventory["resyncPeriod"])
	}
}

func TestSanitizeModuleSettingsHighAvailability(t *testing.T) {
	cfg, err := sanitizeModuleSettings(map[string]any{
		"highAvailability": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	value, ok := cfg["highAvailability"]
	if !ok {
		t.Fatalf("highAvailability key missing in sanitized config: %#v", cfg)
	}
	flag, ok := value.(bool)
	if !ok {
		t.Fatalf("highAvailability expected to be bool, got %T", value)
	}
	if !flag {
		t.Fatalf("highAvailability expected to be true, got false")
	}
}

func TestSanitizeModuleSettingsDeviceApprovalSelector(t *testing.T) {
	cfg, err := sanitizeModuleSettings(map[string]any{
		"deviceApproval": map[string]any{
			"mode": "selector",
			"selector": map[string]any{
				"matchLabels": map[string]any{
					"gpu.deckhouse.io/product": "A100 ",
				},
				"matchExpressions": []any{
					map[string]any{
						"key":      "gpu.deckhouse.io/driver.module.nvidia",
						"operator": "in",
						"values":   []any{" true "},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	selector := cfg["deviceApproval"].(map[string]any)["selector"].(map[string]any)
	expected := map[string]any{
		"matchLabels": map[string]string{
			"gpu.deckhouse.io/product": "A100",
		},
		"matchExpressions": []map[string]any{
			{
				"key":      "gpu.deckhouse.io/driver.module.nvidia",
				"operator": "In",
				"values":   []string{"true"},
			},
		},
	}
	if !reflect.DeepEqual(selector, expected) {
		t.Fatalf("unexpected selector normalization:\nexpected: %#v\nactual:   %#v", expected, selector)
	}
}

func TestSanitizeModuleSettingsDeviceApprovalSelectorMissing(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"deviceApproval": map[string]any{
			"mode": "Selector",
		},
	})
	if err == nil {
		t.Fatal("expected error when selector mode is used without selector")
	}
}

func TestSanitizeModuleSettingsInventoryResync(t *testing.T) {
	cfg, err := sanitizeModuleSettings(map[string]any{
		"inventory": map[string]any{
			"resyncPeriod": "45s",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inventory := cfg["inventory"].(map[string]any)
	if inventory["resyncPeriod"] != "45s" {
		t.Fatalf("unexpected resyncPeriod: %#v", inventory["resyncPeriod"])
	}
}

func TestSanitizeModuleSettingsInventoryInvalid(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"inventory": map[string]any{
			"resyncPeriod": "not-a-duration",
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid resyncPeriod")
	}
}
