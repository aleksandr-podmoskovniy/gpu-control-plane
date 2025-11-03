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

package prepare_internal

import (
	"context"
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

func TestHandlePrepareInternalCreatesMissingMaps(t *testing.T) {
	input, pv := newHookInput(t, map[string]any{})

	if err := handlePrepareInternal(context.Background(), input); err != nil {
		t.Fatalf("handlePrepareInternal returned error: %v", err)
	}

	expected := map[string]struct{}{
		"/gpuControlPlane":                                {},
		"/gpuControlPlane/internal":                       {},
		"/gpuControlPlane/internal/rootCA":                {},
		"/gpuControlPlane/internal/controller":            {},
		"/gpuControlPlane/internal/controller/cert":       {},
		"/gpuControlPlane/internal/customCertificateData": {},
	}

	patches := pv.GetPatches()
	if len(patches) != len(expected) {
		t.Fatalf("expected %d patches, got %d", len(expected), len(patches))
	}

	for _, patch := range patches {
		if _, ok := expected[patch.Path]; !ok {
			t.Fatalf("unexpected patch path %s", patch.Path)
		}
		if patch.Op != "add" {
			t.Fatalf("unexpected operation %s for path %s", patch.Op, patch.Path)
		}
	}
}

func TestHandlePrepareInternalPreservesExistingMaps(t *testing.T) {
	input, pv := newHookInput(t, map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"rootCA": map[string]any{"crt": "value"},
			},
		},
	})

	if err := handlePrepareInternal(context.Background(), input); err != nil {
		t.Fatalf("handlePrepareInternal returned error: %v", err)
	}

	for _, patch := range pv.GetPatches() {
		if patch.Path == "/gpuControlPlane/internal/rootCA" {
			t.Fatalf("expected rootCA map to remain untouched")
		}
	}
}
