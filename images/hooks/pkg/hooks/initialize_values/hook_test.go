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

package initialize_values

import (
	"context"
	"strings"
	"testing"

	pkg "github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"

	"hooks/pkg/settings"
)

func newHookInput(t *testing.T, values map[string]any) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	t.Helper()

	patchable, err := patchablevalues.NewPatchableValues(values)
	if err != nil {
		t.Fatalf("create patchable values: %v", err)
	}

	return &pkg.HookInput{Values: patchable}, patchable
}

func TestHandleInitializeValuesCreatesMissingMaps(t *testing.T) {
	input, patchable := newHookInput(t, map[string]any{})

	if err := handleInitializeValues(context.Background(), input); err != nil {
		t.Fatalf("handleInitializeValues returned error: %v", err)
	}

	patches := patchable.GetPatches()
	expectedPaths := []string{
		settings.ConfigRoot,
		settings.ConfigRoot + ".managedNodes",
		settings.ConfigRoot + ".deviceApproval",
		settings.ConfigRoot + ".scheduling",
		settings.ConfigRoot + ".inventory",
		settings.ConfigRoot + ".runtime",
		settings.ConfigRoot + ".monitoring",
		settings.ConfigRoot + ".images",
		settings.InternalRootPath,
		settings.InternalModuleConfigPath,
		settings.InternalModuleValidationPath,
		settings.InternalModuleConditionsPath,
		settings.InternalBootstrapPath,
		settings.InternalControllerPath,
		settings.InternalControllerPath + ".config",
		settings.InternalControllerCertPath,
		settings.InternalCertificatesPath,
		settings.InternalCertificatesRootPath,
	}
	if len(patches) != len(expectedPaths) {
		t.Fatalf("expected %d patches, got %d", len(expectedPaths), len(patches))
	}

	created := make(map[string]struct{}, len(patches))
	for _, patch := range patches {
		if patch.Op != "add" {
			t.Fatalf("unexpected patch op %q", patch.Op)
		}
		created[patch.Path] = struct{}{}
	}

	for _, path := range expectedPaths {
		slash := "/" + strings.ReplaceAll(path, ".", "/")
		if _, ok := created[slash]; !ok {
			t.Fatalf("expected patch for %s", path)
		}
	}
}

func TestHandleInitializeValuesKeepsExistingMaps(t *testing.T) {
	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"managedNodes": map[string]any{
				"labelKey": "existing",
			},
		},
	}
	input, patchable := newHookInput(t, values)

	if err := handleInitializeValues(context.Background(), input); err != nil {
		t.Fatalf("handleInitializeValues returned error: %v", err)
	}

	patches := patchable.GetPatches()
	for _, patch := range patches {
		if patch.Path == "/"+strings.ReplaceAll(settings.ConfigRoot+".managedNodes", ".", "/") {
			t.Fatalf("expected managedNodes not to be patched")
		}
	}

	managed := values[settings.ConfigRoot].(map[string]any)["managedNodes"].(map[string]any)["labelKey"].(string)
	if managed != "existing" {
		t.Fatalf("expected original managedNodes label to remain, got %q", managed)
	}
}
