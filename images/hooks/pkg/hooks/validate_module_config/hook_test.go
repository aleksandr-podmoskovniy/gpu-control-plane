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

	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
	pkg "github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/patchable-values"
	"github.com/deckhouse/module-sdk/pkg/utils"

	"hooks/pkg/settings"
)

type simpleSnapshot struct {
	payload any
}

func (s simpleSnapshot) UnmarshalTo(v any) error {
	data, err := json.Marshal(s.payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (simpleSnapshot) String() string { return "" }

type simpleSnapshots map[string][]pkg.Snapshot

func (s simpleSnapshots) Get(key string) []pkg.Snapshot {
	return s[key]
}

func decodePatchValue(t *testing.T, op *utils.ValuesPatchOperation) any {
	t.Helper()

	if len(op.Value) == 0 {
		return nil
	}

	var out any
	if err := json.Unmarshal(op.Value, &out); err != nil {
		t.Fatalf("decode patch value for %s: %v", op.Path, err)
	}
	return out
}

func newHookInput(t *testing.T, values map[string]any) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	t.Helper()

	pv, err := patchablevalues.NewPatchableValues(values)
	if err != nil {
		t.Fatalf("new patchable values: %v", err)
	}

	return &pkg.HookInput{
		Values: pv,
	}, pv
}

func TestHandleValidateModuleConfigSetsValues(t *testing.T) {
	input, values := newHookInput(t, map[string]any{
		"global": map[string]any{
			"modules": map[string]any{
				"https": map[string]any{
					"mode": "CertManager",
					"certManager": map[string]any{
						"clusterIssuerName": "global-issuer",
					},
				},
			},
		},
	})

	enabled := true
	input.Snapshots = simpleSnapshots{
		moduleConfigSnapshot: {
			simpleSnapshot{payload: moduleConfigSnapshotPayload{
				Spec: moduleConfigSnapshotSpec{
					Enabled: &enabled,
					Settings: map[string]any{
						"managedNodes": map[string]any{
							"labelKey":         "gpu.deckhouse.io/managed",
							"enabledByDefault": false,
						},
						"deviceApproval": map[string]any{
							"mode": "Automatic",
						},
						"scheduling": map[string]any{
							"defaultStrategy": "Spread",
							"topologyKey":     "topology.kubernetes.io/zone",
						},
						"inventory": map[string]any{
							"resyncPeriod": "45s",
						},
						"https": map[string]any{
							"mode": "CustomCertificate",
							"customCertificate": map[string]any{
								"secretName": "corp-cert",
							},
						},
						"highAvailability": true,
					},
				},
			}},
		},
	}

	if err := handleValidateModuleConfig(context.Background(), input); err != nil {
		t.Fatalf("handleValidateModuleConfig: %v", err)
	}

	patches := filteredPatches(t, values)

	moduleCfg, ok := patches["/gpuControlPlane/internal/moduleConfig"].(map[string]any)
	if !ok {
		t.Fatalf("internal module config payload missing: %#v", patches)
	}
	if enabledFlag, _ := moduleCfg["enabled"].(bool); !enabledFlag {
		t.Fatalf("expected module enabled flag true, got %v", moduleCfg["enabled"])
	}

	settings, ok := moduleCfg["settings"].(map[string]any)
	if !ok {
		t.Fatalf("sanitized settings missing: %#v", moduleCfg)
	}

	assertMap(t, patches, "/gpuControlPlane/managedNodes", func(values map[string]any) {
		if values["enabledByDefault"] != false {
			t.Fatalf("managedNodes.enabledByDefault expected false, got %#v", values["enabledByDefault"])
		}
	})

	assertMap(t, patches, "/gpuControlPlane/deviceApproval", func(values map[string]any) {
		if values["mode"] != "Automatic" {
			t.Fatalf("deviceApproval.mode expected Automatic, got %#v", values["mode"])
		}
	})

	assertMap(t, patches, "/gpuControlPlane/scheduling", func(values map[string]any) {
		if values["topologyKey"] != "topology.kubernetes.io/zone" {
			t.Fatalf("unexpected topologyKey: %#v", values["topologyKey"])
		}
	})

	assertScalar(t, patches, "/gpuControlPlane/inventory/resyncPeriod", "45s")

	assertMap(t, patches, "/gpuControlPlane/https", func(values map[string]any) {
		if values["mode"] != "CustomCertificate" {
			t.Fatalf("unexpected https.mode: %#v", values["mode"])
		}
	})

	if ha, ok := patches["/gpuControlPlane/highAvailability"].(bool); !ok || !ha {
		t.Fatalf("expected highAvailability=true, got %#v", patches["/gpuControlPlane/highAvailability"])
	}

	assertMap(t, patches, "/gpuControlPlane/internal/controller/config", func(values map[string]any) {
		if len(values) == 0 {
			t.Fatalf("controller config not generated")
		}
	})

	if _, exists := settings["https"]; !exists {
		t.Fatalf("sanitized settings must contain https section: %#v", settings)
	}

	if _, ok := patches["/gpuControlPlane/internal/moduleConfigValidation"]; ok {
		t.Fatalf("module validation error should not be set, patches: %#v", patches)
	}
}

func TestHandleValidateModuleConfigErrorRegistersValidation(t *testing.T) {
	input, values := newHookInput(t, map[string]any{})

	orig := moduleConfigFromSnapshotFn
	moduleConfigFromSnapshotFn = func(*pkg.HookInput) (*moduleconfig.State, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { moduleConfigFromSnapshotFn = orig })

	if err := handleValidateModuleConfig(context.Background(), input); err != nil {
		t.Fatalf("handleValidateModuleConfig: %v", err)
	}

	patches := filteredPatches(t, values)
	validation, ok := patches[patchPath(settings.InternalModuleValidationPath)]
	if !ok {
		t.Fatalf("validation payload missing, patches: %#v", patches)
	}
	payload, ok := validation.(map[string]any)
	if !ok {
		t.Fatalf("unexpected validation payload type: %#v", validation)
	}
	if payload["error"] != "boom" {
		t.Fatalf("unexpected validation message: %#v", payload["error"])
	}
}

func TestHandleValidateModuleConfigNoSnapshot(t *testing.T) {
	input, values := newHookInput(t, map[string]any{})
	input.Snapshots = simpleSnapshots{}

	if err := handleValidateModuleConfig(context.Background(), input); err != nil {
		t.Fatalf("handleValidateModuleConfig: %v", err)
	}

	if patches := filteredPatches(t, values); len(patches) != 0 {
		t.Fatalf("expected no patches when ModuleConfig missing, got %#v", patches)
	}
}

func TestHandleValidateModuleConfigUsesGlobalHTTPSFallback(t *testing.T) {
	input, values := newHookInput(t, map[string]any{
		"global": map[string]any{
			"modules": map[string]any{
				"https": map[string]any{
					"mode": "OnlyInURI",
				},
			},
		},
	})

	customState := &moduleconfig.State{
		Enabled:   true,
		Sanitized: map[string]any{},
	}

	orig := moduleConfigFromSnapshotFn
	moduleConfigFromSnapshotFn = func(*pkg.HookInput) (*moduleconfig.State, error) {
		clone := customState.Clone()
		return &clone, nil
	}
	t.Cleanup(func() { moduleConfigFromSnapshotFn = orig })

	if err := handleValidateModuleConfig(context.Background(), input); err != nil {
		t.Fatalf("handleValidateModuleConfig: %v", err)
	}

	patches := filteredPatches(t, values)
	assertMap(t, patches, "/gpuControlPlane/https", func(values map[string]any) {
		if values["mode"] != "OnlyInURI" {
			t.Fatalf("expected global fallback mode OnlyInURI, got %#v", values["mode"])
		}
	})
}

func TestHandleValidateModuleConfigRemovesStaleValues(t *testing.T) {
	initial := map[string]any{
		"gpuControlPlane": map[string]any{
			"managedNodes": map[string]any{"enabledByDefault": true},
			"deviceApproval": map[string]any{
				"mode": "Manual",
			},
			"scheduling": map[string]any{
				"defaultStrategy": "Spread",
			},
			"inventory": map[string]any{
				"resyncPeriod": "30s",
			},
			"https": map[string]any{
				"mode": "CertManager",
			},
			"highAvailability": true,
			"internal": map[string]any{
				"controller": map[string]any{
					"config": map[string]any{"foo": "bar"},
				},
			},
		},
		"global": map[string]any{
			"modules": map[string]any{
				"https": map[string]any{
					"mode": "CustomCertificate",
				},
			},
		},
	}
	input, values := newHookInput(t, initial)

	customState := &moduleconfig.State{
		Enabled:   true,
		Sanitized: map[string]any{},
	}
	orig := moduleConfigFromSnapshotFn
	moduleConfigFromSnapshotFn = func(*pkg.HookInput) (*moduleconfig.State, error) {
		clone := customState.Clone()
		return &clone, nil
	}
	t.Cleanup(func() { moduleConfigFromSnapshotFn = orig })

	if err := handleValidateModuleConfig(context.Background(), input); err != nil {
		t.Fatalf("handleValidateModuleConfig: %v", err)
	}

	removals := make(map[string]struct{})
	for _, op := range values.GetPatches() {
		if op.Op == "remove" {
			removals[op.Path] = struct{}{}
		}
	}

	expected := []string{
		"/gpuControlPlane/managedNodes",
		"/gpuControlPlane/deviceApproval",
		"/gpuControlPlane/scheduling",
		"/gpuControlPlane/inventory",
	}
	for _, path := range expected {
		if _, ok := removals[path]; !ok {
			t.Fatalf("expected remove operation for %s, removals: %#v", path, removals)
		}
	}

	if _, ok := removals["/gpuControlPlane/https"]; !ok {
		t.Fatalf("expected https removal, removals: %#v", removals)
	}
	if _, ok := removals["/gpuControlPlane/highAvailability"]; !ok {
		t.Fatalf("expected highAvailability removal, removals: %#v", removals)
	}
	if _, ok := removals["/gpuControlPlane/internal/controller/config"]; !ok {
		t.Fatalf("expected controller config removal, removals: %#v", removals)
	}

	patches := filteredPatches(t, values)
	moduleCfg, ok := patches["/gpuControlPlane/internal/moduleConfig"].(map[string]any)
	if !ok || len(moduleCfg) != 1 || moduleCfg["enabled"] != true {
		t.Fatalf("module config should contain only enabled flag, got %#v", moduleCfg)
	}
	if _, exists := moduleCfg["settings"]; exists {
		t.Fatalf("settings must be omitted when sanitized empty: %#v", moduleCfg)
	}
}

func TestResolveHTTPSConfigPrefersUserValue(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{})
	user := map[string]any{"mode": "CustomCertificate"}
	if result := resolveHTTPSConfig(input, user); !reflect.DeepEqual(result, user) {
		t.Fatalf("expected user config preserved, got %#v", result)
	}
}

func TestResolveHTTPSConfigGlobalCustomCertificate(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{
		"global": map[string]any{
			"modules": map[string]any{
				"https": map[string]any{
					"mode": "CustomCertificate",
					"customCertificate": map[string]any{
						"secretName": "corp-cert",
					},
				},
			},
		},
	})
	result := resolveHTTPSConfig(input, nil)
	if result == nil || result["mode"] != "CustomCertificate" {
		t.Fatalf("expected custom certificate config, got %#v", result)
	}
}

func TestResolveHTTPSConfigFallsBackToCertManager(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{
		"global": map[string]any{
			"modules": map[string]any{
				"https": map[string]any{
					"mode": "CertManager",
					"certManager": map[string]any{
						"clusterIssuerName": "internal",
					},
				},
			},
		},
	})
	result := resolveHTTPSConfig(input, nil)
	if result == nil || result["mode"] != "CertManager" {
		t.Fatalf("expected cert-manager config, got %#v", result)
	}
}

func TestResolveHTTPSConfigCustomCertificateRequiresSecret(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{
		"global": map[string]any{
			"modules": map[string]any{
				"https": map[string]any{
					"mode": "CustomCertificate",
				},
			},
		},
	})
	if result := resolveHTTPSConfig(input, nil); result != nil {
		t.Fatalf("expected nil when secret missing, got %#v", result)
	}
}

func TestResolveHTTPSConfigUsesDefaultsWhenGlobalsEmpty(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{})

	result := resolveHTTPSConfig(input, nil)
	if result == nil {
		t.Fatalf("expected default https config, got nil")
	}
	if mode := result["mode"]; mode != "CertManager" {
		t.Fatalf("expected default mode CertManager, got %#v", mode)
	}
	certMgr, _ := result["certManager"].(map[string]any)
	if certMgr["clusterIssuerName"] != settings.DefaultHTTPSClusterIssuer {
		t.Fatalf("expected default issuer %q, got %#v", settings.DefaultHTTPSClusterIssuer, certMgr["clusterIssuerName"])
	}
}

func TestResolveHTTPSConfigHandlesGlobalDisabled(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{
		"global": map[string]any{
			"modules": map[string]any{
				"https": map[string]any{
					"mode": "Disabled",
				},
			},
		},
	})

	result := resolveHTTPSConfig(input, nil)
	if result == nil {
		t.Fatalf("expected disabled config map")
	}
	if mode := result["mode"]; mode != "Disabled" {
		t.Fatalf("expected mode Disabled, got %#v", mode)
	}
	if _, exists := result["certManager"]; exists {
		t.Fatalf("disabled mode should not include certManager block: %#v", result)
	}
}

func TestNormalizeHTTPSMode(t *testing.T) {
	cases := map[string]string{
		"":                  "",
		"certmanager":       "CertManager",
		"customcertificate": "CustomCertificate",
		"onlyinuri":         "OnlyInURI",
		"disabled":          "Disabled",
		"unknown":           "",
	}
	for input, expected := range cases {
		if got := normalizeHTTPSMode(input); got != expected {
			t.Fatalf("normalizeHTTPSMode(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestModuleConfigFromSnapshotError(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{})
	input.Snapshots = simpleSnapshots{
		moduleConfigSnapshot: {
			simpleSnapshot{payload: map[string]any{"spec": "invalid"}},
		},
	}

	if _, err := moduleConfigFromSnapshot(input); err == nil {
		t.Fatalf("expected error when snapshot payload invalid")
	}
}

func TestModuleConfigFromSnapshotInvalidSettings(t *testing.T) {
	input, _ := newHookInput(t, map[string]any{})
	input.Snapshots = simpleSnapshots{
		moduleConfigSnapshot: {
			simpleSnapshot{payload: moduleConfigSnapshotPayload{
				Spec: moduleConfigSnapshotSpec{
					Settings: map[string]any{
						"inventory": map[string]any{
							"resyncPeriod": "not-a-duration",
						},
					},
				},
			}},
		},
	}

	if _, err := moduleConfigFromSnapshot(input); err == nil {
		t.Fatalf("expected parse error for invalid inventory resyncPeriod")
	}
}

func TestBuildControllerConfigNilInput(t *testing.T) {
	if result := buildControllerConfig(nil); result != nil {
		t.Fatalf("expected nil result for nil input, got %#v", result)
	}
}

func TestBuildControllerConfigModuleSectionOnly(t *testing.T) {
	cfg := map[string]any{
		"managedNodes": map[string]any{
			"labelKey":         "gpu.deckhouse.io/enabled",
			"enabledByDefault": true,
		},
	}

	result := buildControllerConfig(cfg)
	if result == nil || len(result) != 1 {
		t.Fatalf("expected module section only, got %#v", result)
	}
	module, ok := result["module"].(map[string]any)
	if !ok || module["managedNodes"] == nil {
		t.Fatalf("module section missing managedNodes: %#v", result)
	}
	if _, ok := result["controllers"]; ok {
		t.Fatalf("controllers section must be absent when inventory missing: %#v", result)
	}
}

var structuralPaths = makeStructuralPaths()

func filteredPatches(t *testing.T, values *patchablevalues.PatchableValues) map[string]any {
	t.Helper()

	result := make(map[string]any)
	for _, op := range values.GetPatches() {
		if op.Op != "add" && op.Op != "replace" {
			continue
		}
		value := decodePatchValue(t, op)
		if isStructuralEmpty(op.Path, value) {
			continue
		}
		result[op.Path] = value
	}
	return result
}

func isStructuralEmpty(path string, value any) bool {
	if _, ok := structuralPaths[path]; !ok {
		return false
	}
	m, ok := value.(map[string]any)
	return ok && len(m) == 0
}

func makeStructuralPaths() map[string]struct{} {
	return map[string]struct{}{
		patchPath(settings.ConfigRoot):                    {},
		patchPath(settings.ConfigRoot + ".internal"):      {},
		patchPath(settings.InternalModuleConfigPath):      {},
		patchPath(settings.InternalModuleValidationPath):  {},
		patchPath(settings.InternalModuleConditionsPath):  {},
		patchPath(settings.InternalNodeFeatureRulePath):   {},
		patchPath(settings.InternalControllerPath):        {},
		patchPath(settings.InternalControllerCertPath):    {},
		patchPath(settings.InternalRootCAPath):            {},
		patchPath(settings.InternalCustomCertificatePath): {},
	}
}

func patchPath(path string) string {
	return "/" + strings.ReplaceAll(path, ".", "/")
}

func assertScalar[T comparable](t *testing.T, patches map[string]any, path string, expected T) {
	t.Helper()

	value, ok := patches[path]
	if !ok {
		t.Fatalf("patch %q missing, patches: %#v", path, patches)
	}
	scalar, ok := value.(T)
	if !ok {
		t.Fatalf("patch %q type %T, want %T", path, value, expected)
	}
	if scalar != expected {
		t.Fatalf("patch %q=%#v, want %#v", path, scalar, expected)
	}
}

func assertMap(t *testing.T, patches map[string]any, path string, verify func(map[string]any)) {
	t.Helper()

	value, ok := patches[path]
	if !ok {
		t.Fatalf("patch %q missing, patches: %#v", path, patches)
	}
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("patch %q has unexpected type %T", path, value)
	}
	verify(m)
}
