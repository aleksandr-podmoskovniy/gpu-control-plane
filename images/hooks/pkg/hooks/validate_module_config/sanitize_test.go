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
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	pkg "github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"
	"github.com/deckhouse/module-sdk/testing/mock"

	"hooks/pkg/settings"
)

type invalidJSON struct{}

func (invalidJSON) MarshalJSON() ([]byte, error) {
	return []byte("\"bad\""), nil
}

func newHookInput(t *testing.T, values map[string]any) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	patchable, err := patchablevalues.NewPatchableValues(values)
	if err != nil {
		t.Fatalf("create patchable values: %v", err)
	}
	return &pkg.HookInput{Values: patchable}, patchable
}

func slash(path string) string {
	return "/" + strings.ReplaceAll(path, ".", "/")
}

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

	if _, ok := cfg["https"]; ok {
		t.Fatalf("https must not be present when settings are omitted: %#v", cfg)
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

func TestSanitizeModuleSettingsHTTPSCustomCertificateRequiresSecret(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"https": map[string]any{
			"mode": "CustomCertificate",
			"customCertificate": map[string]any{
				"secretName": " ",
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "https.customCertificate.secretName") {
		t.Fatalf("expected secretName validation error, got %v", err)
	}
}

func TestSanitizeModuleSettingsHTTPSCustomCertificateMissingBlock(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"https": map[string]any{
			"mode": "CustomCertificate",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "https.customCertificate.secretName") {
		t.Fatalf("expected missing secretName validation error, got %v", err)
	}
}

func TestSanitizeModuleSettingsHTTPSCustomCertificate(t *testing.T) {
	cfg, err := sanitizeModuleSettings(map[string]any{
		"https": map[string]any{
			"mode": "CustomCertificate",
			"customCertificate": map[string]any{
				"secretName": "company-ca",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	httpsCfg := cfg["https"].(map[string]any)
	if httpsCfg["mode"] != "CustomCertificate" {
		t.Fatalf("unexpected https.mode: %v", httpsCfg["mode"])
	}
	custom := httpsCfg["customCertificate"].(map[string]any)
	if custom["secretName"] != "company-ca" {
		t.Fatalf("unexpected secretName: %v", custom["secretName"])
	}
	if _, ok := httpsCfg["certManager"]; ok {
		t.Fatalf("certManager section must be absent in CustomCertificate mode: %#v", httpsCfg)
	}
}

func TestSanitizeModuleSettingsHTTPSEmptyProvidesDefaults(t *testing.T) {
	cfg, err := sanitizeModuleSettings(map[string]any{
		"https": map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	httpsCfg := cfg["https"].(map[string]any)
	if httpsCfg["mode"] != "CertManager" {
		t.Fatalf("expected default mode CertManager, got %#v", httpsCfg["mode"])
	}
	cm := httpsCfg["certManager"].(map[string]any)
	if cm["clusterIssuerName"] != settings.DefaultHTTPSClusterIssuer {
		t.Fatalf("expected default issuer, got %#v", cm["clusterIssuerName"])
	}
}

func TestSanitizeModuleSettingsHTTPSDisabled(t *testing.T) {
	cfg, err := sanitizeModuleSettings(map[string]any{
		"https": map[string]any{"mode": "Disabled"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	httpsCfg := cfg["https"].(map[string]any)
	if httpsCfg["mode"] != "Disabled" {
		t.Fatalf("unexpected https.mode: %v", httpsCfg["mode"])
	}
	if _, ok := httpsCfg["certManager"]; ok {
		t.Fatalf("certManager section must not be present for Disabled mode: %#v", httpsCfg)
	}
	if _, ok := httpsCfg["customCertificate"]; ok {
		t.Fatalf("customCertificate section must not be present for Disabled mode: %#v", httpsCfg)
	}
}

func TestSanitizeModuleSettingsHTTPSOnlyInURI(t *testing.T) {
	cfg, err := sanitizeModuleSettings(map[string]any{
		"https": map[string]any{"mode": "OnlyInURI"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h := cfg["https"].(map[string]any)
	if h["mode"] != "OnlyInURI" {
		t.Fatalf("unexpected https.mode: %v", h["mode"])
	}
	if _, ok := h["certManager"]; ok {
		t.Fatalf("certManager section must not be present for OnlyInURI mode: %#v", h)
	}
	if _, ok := h["customCertificate"]; ok {
		t.Fatalf("customCertificate section must not be present for OnlyInURI mode: %#v", h)
	}
}

func TestSanitizeModuleSettingsHTTPSCertManagerOverridesIssuer(t *testing.T) {
	cfg, err := sanitizeModuleSettings(map[string]any{
		"https": map[string]any{
			"mode": "CertManager",
			"certManager": map[string]any{
				"clusterIssuerName": "internal-issuer",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	httpsCfg := cfg["https"].(map[string]any)
	if httpsCfg["mode"] != "CertManager" {
		t.Fatalf("unexpected https.mode: %v", httpsCfg["mode"])
	}
	issuer := httpsCfg["certManager"].(map[string]any)["clusterIssuerName"]
	if issuer != "internal-issuer" {
		t.Fatalf("expected overridden issuer, got %v", issuer)
	}
}

func TestSanitizeModuleSettingsHTTPSUnknownMode(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"https": map[string]any{"mode": "TotallySecure"},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown https.mode") {
		t.Fatalf("expected unknown mode error, got %v", err)
	}
}

func TestSanitizeModuleSettingsHTTPSInvalidPayload(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"https": "invalid",
	})
	if err == nil || !strings.Contains(err.Error(), "decode https settings") {
		t.Fatalf("expected decode error, got %v", err)
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

func TestSanitizeManagedNodesCustom(t *testing.T) {
	cfg, err := sanitizeModuleSettings(map[string]any{
		"managedNodes": map[string]any{
			"labelKey":         " custom.label ",
			"enabledByDefault": false,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	managed := cfg["managedNodes"].(map[string]any)
	if managed["labelKey"] != "custom.label" {
		t.Fatalf("expected trimmed labelKey, got %#v", managed["labelKey"])
	}
	if managed["enabledByDefault"] != false {
		t.Fatalf("expected enabledByDefault=false, got %#v", managed["enabledByDefault"])
	}
}

func TestSanitizeManagedNodesDecodeError(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"managedNodes": "should-be-object",
	})
	if err == nil {
		t.Fatal("expected decode error for managedNodes")
	}
}

func TestSanitizeDeviceApprovalUnknownMode(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"deviceApproval": map[string]any{
			"mode": "unsupported",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown deviceApproval.mode") {
		t.Fatalf("expected unknown mode error, got %v", err)
	}
}

func TestSanitizeDeviceApprovalDecodeError(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"deviceApproval": "not-a-map",
	})
	if err == nil {
		t.Fatal("expected decode error for deviceApproval")
	}
}

func TestSanitizeDeviceApprovalSelectorValidation(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"deviceApproval": map[string]any{
			"mode": "Selector",
			"selector": map[string]any{
				"matchLabels": map[string]any{
					"": "value",
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "matchLabels keys and values must be non-empty") {
		t.Fatalf("expected matchLabels error, got %v", err)
	}

	_, err = sanitizeModuleSettings(map[string]any{
		"deviceApproval": map[string]any{
			"mode": "Selector",
			"selector": map[string]any{
				"matchExpressions": []any{
					map[string]any{
						"key":    "gpu",
						"values": []any{"true"},
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "operator must be set") {
		t.Fatalf("expected operator error, got %v", err)
	}

	_, err = sanitizeModuleSettings(map[string]any{
		"deviceApproval": map[string]any{
			"mode": "Selector",
			"selector": map[string]any{
				"matchExpressions": []any{
					map[string]any{
						"key":      "gpu",
						"operator": "Gt",
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported selector operator") {
		t.Fatalf("expected unsupported operator error, got %v", err)
	}

	_, err = sanitizeModuleSettings(map[string]any{
		"deviceApproval": map[string]any{
			"mode": "Selector",
			"selector": map[string]any{
				"matchExpressions": []any{
					map[string]any{
						"key":      "",
						"operator": "In",
						"values":   []any{"value"},
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "key must be set") {
		t.Fatalf("expected key required error, got %v", err)
	}

	_, err = sanitizeModuleSettings(map[string]any{
		"deviceApproval": map[string]any{
			"mode": "Selector",
			"selector": map[string]any{
				"matchExpressions": []any{
					map[string]any{
						"key":      "gpu",
						"operator": "In",
						"values":   []any{},
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "requires non-empty values") {
		t.Fatalf("expected values required error, got %v", err)
	}

	_, err = sanitizeModuleSettings(map[string]any{
		"deviceApproval": map[string]any{
			"mode": "Selector",
			"selector": map[string]any{
				"matchExpressions": []any{
					map[string]any{
						"key":      "gpu",
						"operator": "Exists",
						"values":   []any{"value"},
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "does not accept values") {
		t.Fatalf("expected values forbidden error, got %v", err)
	}
}

func TestSanitizeSelectorRequiresCriteria(t *testing.T) {
	_, err := sanitizeSelector(&labelSelector{})
	if err == nil || !strings.Contains(err.Error(), "must define matchLabels or matchExpressions") {
		t.Fatalf("expected selector criteria error, got %v", err)
	}
}

func TestNormalizeHelpers(t *testing.T) {
	if normalizeMode("Manual") != "Manual" || normalizeMode("automatic") != "Automatic" || normalizeMode("selector") != "Selector" {
		t.Fatal("normalizeMode did not normalize known values")
	}
	if normalizeMode("unknown") != "" || normalizeMode("   ") != "" {
		t.Fatal("normalizeMode must return empty string for unknown values")
	}

	if normalizeStrategy("binpack") != "BinPack" || normalizeStrategy("spread") != "Spread" || normalizeStrategy("") != "" {
		t.Fatal("normalizeStrategy failed to normalize values")
	}
	if normalizeStrategy("unknown") != "" {
		t.Fatal("normalizeStrategy must return empty string for unknown strategy")
	}

	if normalizeOperator("in") != "In" || normalizeOperator("NOTIN") != "NotIn" || normalizeOperator("exists") != "Exists" || normalizeOperator("doesNotExist") != "DoesNotExist" {
		t.Fatal("normalizeOperator failed to normalize values")
	}
	if normalizeOperator("invalid") != "" {
		t.Fatal("normalizeOperator must return empty string for unknown operator")
	}
}

func TestSanitizeModuleSettingsMarshalError(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"invalid": make(chan int),
	})
	if err == nil || !strings.Contains(err.Error(), "encode ModuleConfig settings") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}

func TestSanitizeModuleSettingsDecodeError(t *testing.T) {
	original := jsonUnmarshal
	jsonUnmarshal = func([]byte, any) error { return errors.New("boom") }
	defer func() { jsonUnmarshal = original }()

	_, err := sanitizeModuleSettings(map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "decode ModuleConfig settings") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestSanitizeModuleSettingsHighAvailabilityDecodeError(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"highAvailability": "not-bool",
	})
	if err == nil || !strings.Contains(err.Error(), "decode ModuleConfig field") {
		t.Fatalf("expected highAvailability decode error, got %v", err)
	}
}

func TestSanitizeInventoryErrors(t *testing.T) {
	_, err := sanitizeInventory(json.RawMessage("\"oops\""))
	if err == nil || !strings.Contains(err.Error(), "decode inventory settings") {
		t.Fatalf("expected decode error, got %v", err)
	}

	_, err = sanitizeInventory(json.RawMessage(`{"resyncPeriod":"not-a-duration"}`))
	if err == nil || !strings.Contains(err.Error(), "parse inventory.resyncPeriod") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestSanitizeInventoryDefaultDurationError(t *testing.T) {
	original := defaultInventoryResyncPeriod
	defaultInventoryResyncPeriod = "bad"
	defer func() { defaultInventoryResyncPeriod = original }()

	_, err := sanitizeInventory(nil)
	if err == nil || !strings.Contains(err.Error(), "parse inventory.resyncPeriod") {
		t.Fatalf("expected parse error for default period, got %v", err)
	}
}

func TestSanitizeSchedulingTopologyRequirement(t *testing.T) {
	cfg, err := sanitizeModuleSettings(map[string]any{
		"scheduling": map[string]any{
			"defaultStrategy": "Spread",
			"topologyKey":     " ",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scheduling := cfg["scheduling"].(map[string]any)
	if scheduling["topologyKey"] != settings.DefaultSchedulingTopology {
		t.Fatalf("expected topology key to fall back to default, got %v", scheduling["topologyKey"])
	}
}

func TestSanitizeSchedulingTopologyError(t *testing.T) {
	original := defaultSchedulingTopology
	defaultSchedulingTopology = ""
	defer func() { defaultSchedulingTopology = original }()

	_, err := sanitizeModuleSettings(map[string]any{
		"scheduling": map[string]any{
			"defaultStrategy": "Spread",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "topologyKey must be set") {
		t.Fatalf("expected topology error, got %v", err)
	}
}

func TestModuleConfigFromSnapshotStates(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{})
	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(string) []pkg.Snapshot { return nil })
	input.Snapshots = snapshots

	state, err := moduleConfigFromSnapshot(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil state, got %#v", state)
	}

	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(any) error { return errors.New("boom") })
	snapshots.GetMock.Set(func(string) []pkg.Snapshot { return []pkg.Snapshot{snapshot} })

	if _, err = moduleConfigFromSnapshot(input); err == nil || !strings.Contains(err.Error(), "decode ModuleConfig") {
		t.Fatalf("expected decode error, got %v", err)
	}

	enabled := true
	snapshot.UnmarshalToMock.Set(func(target any) error {
		payload := target.(*moduleConfigSnapshotPayload)
		payload.Spec.Enabled = &enabled
		payload.Spec.Settings = map[string]any{
			"managedNodes": map[string]any{
				"labelKey":         " gpu.node/enabled ",
				"enabledByDefault": false,
			},
			"inventory": map[string]any{
				"resyncPeriod": "45s",
			},
		}
		return nil
	})

	snapshots.GetMock.Set(func(string) []pkg.Snapshot { return []pkg.Snapshot{snapshot} })
	state, err = moduleConfigFromSnapshot(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state == nil || !state.Enabled {
		t.Fatalf("expected enabled state, got %#v", state)
	}
	managed := state.Config["managedNodes"].(map[string]any)
	if managed["labelKey"] != "gpu.node/enabled" {
		t.Fatalf("expected sanitized labelKey, got %v", managed["labelKey"])
	}
}

func TestRegisterValidationError(t *testing.T) {
	input, patchable := newHookInput(t, map[string]any{})
	registerValidationError(input, errors.New("failure"))

	patches := patchable.GetPatches()
	if len(patches) != 1 {
		t.Fatalf("expected single patch, got %d", len(patches))
	}
	if patches[0].Path != "/"+strings.ReplaceAll(settings.InternalModuleValidationPath, ".", "/") {
		t.Fatalf("unexpected patch path: %s", patches[0].Path)
	}
}

func TestBuildControllerConfig(t *testing.T) {
	if buildControllerConfig(nil) != nil {
		t.Fatal("expected nil config for nil input")
	}

	if buildControllerConfig(map[string]any{}) != nil {
		t.Fatal("expected nil config for empty input")
	}

	config := map[string]any{
		"inventory":      map[string]any{"resyncPeriod": "60s"},
		"managedNodes":   map[string]any{"labelKey": "gpu"},
		"deviceApproval": map[string]any{"mode": "Manual"},
		"scheduling":     map[string]any{"defaultStrategy": "Spread"},
	}

	result := buildControllerConfig(config)
	controllers, ok := result["controllers"].(map[string]any)
	if !ok {
		t.Fatalf("expected controllers section, got %#v", result)
	}
	gpuInventory := controllers["gpuInventory"].(map[string]any)
	if gpuInventory["resyncPeriod"] != "60s" {
		t.Fatalf("unexpected resyncPeriod: %v", gpuInventory["resyncPeriod"])
	}

	module, ok := result["module"].(map[string]any)
	if !ok {
		t.Fatalf("expected module section, got %#v", result)
	}
	if _, ok := module["managedNodes"]; !ok {
		t.Fatalf("managedNodes missing from module section: %#v", module)
	}
}

func TestHandleValidateModuleConfigError(t *testing.T) {
	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"managedNodes":   map[string]any{"existing": true},
			"deviceApproval": map[string]any{"mode": "Manual"},
			"internal": map[string]any{
				"moduleConfig":           map[string]any{"enabled": false},
				"moduleConfigValidation": map[string]any{"error": "old"},
				"controller":             map[string]any{"config": map[string]any{"foo": "bar"}},
			},
		},
	}

	input, patchable := newHookInput(t, values)
	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(target any) error {
		payload := target.(*moduleConfigSnapshotPayload)
		payload.Spec.Settings = map[string]any{
			"deviceApproval": "invalid",
		}
		return nil
	})
	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(string) []pkg.Snapshot { return []pkg.Snapshot{snapshot} })
	input.Snapshots = snapshots

	if err := handleValidateModuleConfig(context.Background(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	patches := patchable.GetPatches()
	ops := map[string]string{}
	for _, patch := range patches {
		ops[patch.Path] = patch.Op
	}
	if ops[slash(settings.InternalModuleValidationPath)] != "add" {
		t.Fatalf("expected validation error to be set, patches: %#v", patches)
	}
	if ops[slash(settings.InternalModuleConfigPath)] != "remove" {
		t.Fatalf("expected module config removal, patches: %#v", patches)
	}
	if ops[slash(settings.InternalControllerPath+".config")] != "remove" {
		t.Fatalf("expected controller config removal, patches: %#v", patches)
	}
}
func TestHandleValidateModuleConfigDisabled(t *testing.T) {
	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"managedNodes":   map[string]any{"keep": true},
			"deviceApproval": map[string]any{"mode": "Manual"},
			"internal": map[string]any{
				"moduleConfig":           map[string]any{"enabled": true},
				"moduleConfigValidation": map[string]any{"error": "old"},
				"controller":             map[string]any{"config": map[string]any{"foo": "bar"}},
			},
		},
	}
	input, patchable := newHookInput(t, values)
	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(string) []pkg.Snapshot { return nil })
	input.Snapshots = snapshots

	if err := handleValidateModuleConfig(context.Background(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	patches := patchable.GetPatches()
	ops := map[string]string{}
	for _, patch := range patches {
		ops[patch.Path] = patch.Op
	}
	if ops[slash(settings.InternalModuleConfigPath)] != "remove" {
		t.Fatalf("expected module config removal, patches: %#v", patches)
	}
	if ops[slash(settings.InternalModuleValidationPath)] != "remove" {
		t.Fatalf("expected validation removal, patches: %#v", patches)
	}
	if ops[slash(settings.InternalControllerPath+".config")] != "remove" {
		t.Fatalf("expected controller config removal, patches: %#v", patches)
	}
}

func TestHandleValidateModuleConfigWithoutHighAvailability(t *testing.T) {
	original := moduleConfigFromSnapshotFn
	t.Cleanup(func() { moduleConfigFromSnapshotFn = original })

	moduleConfigFromSnapshotFn = func(*pkg.HookInput) (*moduleConfigState, error) {
		return &moduleConfigState{
			Enabled: true,
			Config:  map[string]any{},
		}, nil
	}

	input, patchable := newHookInput(t, map[string]any{
		settings.ConfigRoot: map[string]any{
			"highAvailability": true,
			"internal": map[string]any{
				"moduleConfig": map[string]any{},
			},
		},
	})

	if err := handleValidateModuleConfig(context.Background(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	patches := patchable.GetPatches()
	removed := false
	for _, patch := range patches {
		if patch.Op == "remove" && patch.Path == slash(settings.ConfigRoot+".highAvailability") {
			removed = true
		}
	}
	if !removed {
		t.Fatalf("expected highAvailability removal when absent in state, patches: %#v", patches)
	}
}
func TestHandleValidateModuleConfigSuccess(t *testing.T) {
	enabled := true
	inputValues := map[string]any{
		"global": map[string]any{
			"modules": map[string]any{
				"https": map[string]any{
					"mode": "CustomCertificate",
					"customCertificate": map[string]any{
						"secretName": "corp-ca",
					},
				},
			},
		},
	}
	input, patchable := newHookInput(t, inputValues)
	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(target any) error {
		payload := target.(*moduleConfigSnapshotPayload)
		payload.Spec.Enabled = &enabled
		payload.Spec.Settings = map[string]any{
			"managedNodes":     map[string]any{"labelKey": "gpu.deckhouse.io/enabled"},
			"deviceApproval":   map[string]any{"mode": "Manual"},
			"scheduling":       map[string]any{"defaultStrategy": "Spread"},
			"inventory":        map[string]any{"resyncPeriod": "45s"},
			"highAvailability": true,
		}
		return nil
	})
	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(string) []pkg.Snapshot { return []pkg.Snapshot{snapshot} })
	input.Snapshots = snapshots

	if err := handleValidateModuleConfig(context.Background(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	patches := patchable.GetPatches()
	paths := make(map[string]json.RawMessage)
	for _, patch := range patches {
		if patch.Op == "add" {
			paths[patch.Path] = patch.Value
		}
	}
	if _, ok := paths[slash(settings.ConfigRoot+".managedNodes")]; !ok {
		t.Fatalf("expected managedNodes to be set, patches: %#v", patches)
	}
	if _, ok := paths[slash(settings.ConfigRoot+".highAvailability")]; !ok {
		t.Fatalf("expected highAvailability to be set, patches: %#v", patches)
	}
	payload, ok := paths[slash(settings.ConfigRoot+".https")]
	if !ok {
		t.Fatalf("expected https config to be set, patches: %#v", patches)
	}

	var httpsCfg map[string]any
	if err := json.Unmarshal(payload, &httpsCfg); err != nil {
		t.Fatalf("decode https payload: %v", err)
	}
	if httpsCfg["mode"] != "CustomCertificate" {
		t.Fatalf("expected custom certificate mode, got %#v", httpsCfg["mode"])
	}
	custom := httpsCfg["customCertificate"].(map[string]any)
	if custom["secretName"] != "corp-ca" {
		t.Fatalf("expected secretName corp-ca, got %#v", custom["secretName"])
	}
	if _, ok := paths[slash(settings.InternalControllerPath+".config")]; !ok {
		t.Fatalf("expected controller config patch, patches: %#v", patches)
	}
}

func TestHandleValidateModuleConfigInvalidHighAvailability(t *testing.T) {
	original := moduleConfigFromSnapshotFn
	t.Cleanup(func() { moduleConfigFromSnapshotFn = original })

	moduleConfigFromSnapshotFn = func(*pkg.HookInput) (*moduleConfigState, error) {
		return &moduleConfigState{
			Enabled: true,
			Config:  map[string]any{"highAvailability": "invalid"},
		}, nil
	}

	input, patchable := newHookInput(t, map[string]any{
		settings.ConfigRoot: map[string]any{
			"highAvailability": true,
		},
	})

	if err := handleValidateModuleConfig(context.Background(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	patches := patchable.GetPatches()
	removed := false
	for _, patch := range patches {
		if patch.Op == "remove" && patch.Path == slash(settings.ConfigRoot+".highAvailability") {
			removed = true
		}
	}
	if !removed {
		for _, patch := range patches {
			t.Logf("patch: %s %s", patch.Op, patch.Path)
		}
		t.Fatalf("expected highAvailability to be removed, patches: %#v", patches)
	}
}

func TestHandleValidateModuleConfigRemovesHTTPSWhenUnavailable(t *testing.T) {
	original := moduleConfigFromSnapshotFn
	t.Cleanup(func() { moduleConfigFromSnapshotFn = original })

	moduleConfigFromSnapshotFn = func(*pkg.HookInput) (*moduleConfigState, error) {
		return &moduleConfigState{Enabled: true, Config: map[string]any{}}, nil
	}

	input, patchable := newHookInput(t, map[string]any{
		settings.ConfigRoot: map[string]any{
			"https": map[string]any{"mode": "CertManager"},
		},
		"global": map[string]any{
			"modules": map[string]any{
				"https": map[string]any{
					"mode": "CustomCertificate",
				},
			},
		},
	})

	if err := handleValidateModuleConfig(context.Background(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	removed := false
	for _, patch := range patchable.GetPatches() {
		if patch.Op == "remove" && patch.Path == slash(settings.ConfigRoot+".https") {
			removed = true
		}
	}
	if !removed {
		t.Fatalf("expected https removal patch, got %#v", patchable.GetPatches())
	}
}

func TestHandleValidateModuleConfigWithUserHTTPS(t *testing.T) {
	original := moduleConfigFromSnapshotFn
	t.Cleanup(func() { moduleConfigFromSnapshotFn = original })

	moduleConfigFromSnapshotFn = func(*pkg.HookInput) (*moduleConfigState, error) {
		return &moduleConfigState{
			Enabled: true,
			Config: map[string]any{
				"https": map[string]any{
					"mode": "Disabled",
				},
			},
		}, nil
	}

	input, patchable := newHookInput(t, map[string]any{})

	if err := handleValidateModuleConfig(context.Background(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, patch := range patchable.GetPatches() {
		if patch.Op == "add" && patch.Path == slash(settings.ConfigRoot+".https") {
			var payload map[string]any
			if err := json.Unmarshal(patch.Value, &payload); err != nil {
				t.Fatalf("decode https payload: %v", err)
			}
			if payload["mode"] != "Disabled" {
				t.Fatalf("expected Disabled mode, got %#v", payload["mode"])
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected https patch, got %#v", patchable.GetPatches())
	}
}

func TestResolveHTTPSConfigDefaults(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{})

	cfg := resolveHTTPSConfig(input, nil)
	if cfg["mode"] != "CertManager" {
		t.Fatalf("expected default mode CertManager, got %#v", cfg["mode"])
	}
	cm := cfg["certManager"].(map[string]any)
	if cm["clusterIssuerName"] != settings.DefaultHTTPSClusterIssuer {
		t.Fatalf("expected default issuer, got %#v", cm["clusterIssuerName"])
	}
}

func TestResolveHTTPSConfigGlobalOnlyInURI(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{
		"global": map[string]any{
			"modules": map[string]any{
				"https": map[string]any{
					"mode": "OnlyInURI",
				},
			},
		},
	})

	cfg := resolveHTTPSConfig(input, nil)
	if cfg["mode"] != "OnlyInURI" {
		t.Fatalf("expected OnlyInURI mode, got %#v", cfg["mode"])
	}
	if _, ok := cfg["certManager"]; ok {
		t.Fatalf("certManager section must be empty for OnlyInURI")
	}
}

func TestResolveHTTPSConfigCustomCertificateMissingSecret(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{
		"global": map[string]any{
			"modules": map[string]any{
				"https": map[string]any{
					"mode": "CustomCertificate",
				},
			},
		},
	})

	if cfg := resolveHTTPSConfig(input, nil); cfg != nil {
		t.Fatalf("expected nil config without secret, got %#v", cfg)
	}
}

func TestResolveHTTPSConfigPrefersUserConfig(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{})
	user := map[string]any{
		"mode": "Disabled",
	}

	cfg := resolveHTTPSConfig(input, user)
	if cfg["mode"] != "Disabled" {
		t.Fatalf("expected user config to be returned, got %#v", cfg["mode"])
	}
}

func TestResolveHTTPSConfigGlobalDisabled(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{
		"global": map[string]any{
			"modules": map[string]any{
				"https": map[string]any{
					"mode": "Disabled",
				},
			},
		},
	})

	cfg := resolveHTTPSConfig(input, nil)
	if cfg["mode"] != "Disabled" {
		t.Fatalf("expected Disabled mode, got %#v", cfg["mode"])
	}
	if len(cfg) != 1 {
		t.Fatalf("unexpected extra fields in cfg: %#v", cfg)
	}
}
func TestSanitizeSchedulingErrors(t *testing.T) {
	_, err := sanitizeModuleSettings(map[string]any{
		"scheduling": "not-a-map",
	})
	if err == nil {
		t.Fatal("expected decode error for scheduling")
	}

	_, err = sanitizeModuleSettings(map[string]any{
		"scheduling": map[string]any{
			"defaultStrategy": "RoundRobin",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown scheduling.defaultStrategy") {
		t.Fatalf("expected unknown strategy error, got %v", err)
	}

	cfg, err := sanitizeModuleSettings(map[string]any{
		"scheduling": map[string]any{
			"defaultStrategy": "Spread",
			"topologyKey":     " ",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	scheduling := cfg["scheduling"].(map[string]any)
	if scheduling["topologyKey"] != settings.DefaultSchedulingTopology {
		t.Fatalf("expected topology to fall back to default, got %#v", scheduling["topologyKey"])
	}
}

func TestSanitizeSelectorNil(t *testing.T) {
	_, err := sanitizeSelector(nil)
	if err == nil {
		t.Fatal("expected error for nil selector")
	}
}
