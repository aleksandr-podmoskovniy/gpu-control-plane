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

	if managed := patches["/gpuControlPlane/managedNodes"].(map[string]any); managed["enabledByDefault"] != false {
		t.Fatalf("managedNodes.enabledByDefault expected false, got %#v", managed["enabledByDefault"])
	}

	if approval := patches["/gpuControlPlane/deviceApproval"].(map[string]any); approval["mode"] != "Automatic" {
		t.Fatalf("deviceApproval.mode expected Automatic, got %#v", approval["mode"])
	}

	if scheduling := patches["/gpuControlPlane/scheduling"].(map[string]any); scheduling["topologyKey"] != "topology.kubernetes.io/zone" {
		t.Fatalf("unexpected topologyKey: %#v", scheduling["topologyKey"])
	}

	if inventory := patches["/gpuControlPlane/inventory"].(map[string]any); inventory["resyncPeriod"] != "45s" {
		t.Fatalf("unexpected inventory.resyncPeriod: %#v", inventory["resyncPeriod"])
	}

	if https := patches["/gpuControlPlane/https"].(map[string]any); https["mode"] != "CustomCertificate" {
		t.Fatalf("unexpected https.mode: %#v", https["mode"])
	}

	if ha, _ := patches["/gpuControlPlane/highAvailability"].(bool); !ha {
		t.Fatalf("expected highAvailability=true, got %v", patches["/gpuControlPlane/highAvailability"])
	}

	if controllerCfg := patches["/gpuControlPlane/internal/controller/config"].(map[string]any); controllerCfg == nil {
		t.Fatalf("controller config not generated")
	}

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
	cfg, ok := patches["/gpuControlPlane/https"].(map[string]any)
	if !ok {
		t.Fatalf("https patch missing, patches: %#v", patches)
	}
	if cfg["mode"] != "OnlyInURI" {
		t.Fatalf("expected global fallback mode OnlyInURI, got %#v", cfg["mode"])
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
		patchPath(settings.InternalBootstrapPath):         {},
		patchPath(settings.InternalControllerPath):        {},
		patchPath(settings.InternalControllerCertPath):    {},
		patchPath(settings.InternalRootCAPath):            {},
		patchPath(settings.InternalCustomCertificatePath): {},
	}
}

func patchPath(path string) string {
	return "/" + strings.ReplaceAll(path, ".", "/")
}
