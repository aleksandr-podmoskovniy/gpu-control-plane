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

package tls_certificates_controller

import (
	"strings"
	"testing"

	pkg "github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"

	"hooks/pkg/settings"
)

func newTLSHookInput(t *testing.T, values map[string]any) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	t.Helper()

	patchable, err := patchablevalues.NewPatchableValues(values)
	if err != nil {
		t.Fatalf("create patchable values: %v", err)
	}

	return &pkg.HookInput{Values: patchable}, patchable
}

func TestEnsureControllerTLSValuesCreatesMissingMaps(t *testing.T) {
	input, patchable := newTLSHookInput(t, map[string]any{})

	if !ensureControllerTLSValues(input) {
		t.Fatal("expected ensureControllerTLSValues to return true")
	}

	patches := patchable.GetPatches()
	expectedPaths := []string{
		settings.ConfigRoot,
		settings.InternalRootPath,
		settings.InternalControllerPath,
		settings.InternalControllerCertPath,
		settings.InternalCertificatesPath,
		settings.InternalCertificatesRootPath,
	}
	if len(patches) != len(expectedPaths) {
		t.Fatalf("expected patches for TLS maps, got %d", len(patches))
	}

	expected := make(map[string]struct{}, len(expectedPaths))
	for _, path := range expectedPaths {
		expected["/"+strings.ReplaceAll(path, ".", "/")] = struct{}{}
	}
	for _, patch := range patches {
		if patch.Op != "add" {
			t.Fatalf("unexpected patch op %q", patch.Op)
		}
		if _, ok := expected[patch.Path]; !ok {
			t.Fatalf("unexpected patch path %s", patch.Path)
		}
		delete(expected, patch.Path)
	}
	if len(expected) != 0 {
		t.Fatalf("missing patches for paths: %v", expected)
	}
}

func TestEnsureControllerTLSValuesIsNoopWhenAlreadyPresent(t *testing.T) {
	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"controller": map[string]any{
					"cert": map[string]any{},
				},
				"certificates": map[string]any{
					"root": map[string]any{},
				},
			},
		},
	}
	input, patchable := newTLSHookInput(t, values)

	if !ensureControllerTLSValues(input) {
		t.Fatal("expected ensureControllerTLSValues to return true")
	}
	if len(patchable.GetPatches()) != 0 {
		t.Fatalf("expected no patches when maps already present, got %d", len(patchable.GetPatches()))
	}
}
