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

package internalvalues

import (
	"strings"
	"testing"

	pkg "github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"

	"hooks/pkg/settings"
)

func newHookInput(t *testing.T, values map[string]any) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	t.Helper()

	pv, err := patchablevalues.NewPatchableValues(values)
	if err != nil {
		t.Fatalf("create patchable values: %v", err)
	}

	return &pkg.HookInput{Values: pv}, pv
}

func TestEnsureCreatesInternalMaps(t *testing.T) {
	input, pv := newHookInput(t, map[string]any{})

	Ensure(input)

	expected := map[string]struct{}{
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

	patches := pv.GetPatches()
	if len(patches) != len(expected) {
		t.Fatalf("expected %d patches, got %d", len(expected), len(patches))
	}

	for _, patch := range patches {
		if patch.Op != "add" {
			t.Fatalf("expected add operation, got %s", patch.Op)
		}
		if _, ok := expected[patch.Path]; !ok {
			t.Fatalf("unexpected patch path %s", patch.Path)
		}
	}
}

func TestEnsureKeepsExistingObjects(t *testing.T) {
	input, pv := newHookInput(t, map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"rootCA": map[string]any{"crt": "existing"},
			},
		},
	})

	Ensure(input)

	for _, patch := range pv.GetPatches() {
		if patch.Path == patchPath(settings.InternalRootCAPath) {
			t.Fatalf("expected root CA to remain untouched")
		}
	}
}

func patchPath(path string) string {
	return "/" + strings.ReplaceAll(path, ".", "/")
}
