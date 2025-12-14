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

package tls_certificates_metrics_proxy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/patchable-values"
	"github.com/deckhouse/module-sdk/pkg/utils"

	"hooks/pkg/settings"
)

func newHookInput(t *testing.T, values map[string]any) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	t.Helper()

	pv, err := patchablevalues.NewPatchableValues(values)
	if err != nil {
		t.Fatalf("new patchable values: %v", err)
	}
	return &pkg.HookInput{Values: pv}, pv
}

func patchPath(path string) string {
	return "/" + strings.ReplaceAll(path, ".", "/")
}

func decodePatchValue(t *testing.T, op *utils.ValuesPatchOperation) any {
	t.Helper()
	if len(op.Value) == 0 {
		return nil
	}

	var out any
	if err := json.Unmarshal(op.Value, &out); err != nil {
		t.Fatalf("decode patch value: %v", err)
	}
	return out
}

func TestBeforeHookCheck(t *testing.T) {
	t.Run("module disabled", func(t *testing.T) {
		input, pv := newHookInput(t, map[string]any{})
		if beforeHookCheck(input) || len(pv.GetPatches()) != 0 {
			t.Fatalf("expected false when module config missing")
		}
	})

	t.Run("metrics missing", func(t *testing.T) {
		input, pv := newHookInput(t, map[string]any{
			settings.ConfigRoot: map[string]any{
				"internal": map[string]any{
					"moduleConfig": map[string]any{"enabled": true},
				},
			},
		})
		if beforeHookCheck(input) || len(pv.GetPatches()) != 0 {
			t.Fatalf("expected false when metrics section missing")
		}
	})

	t.Run("ensures cert structure", func(t *testing.T) {
		input, pv := newHookInput(t, map[string]any{
			settings.ConfigRoot: map[string]any{
				"internal": map[string]any{
					"moduleConfig": map[string]any{"enabled": true},
					"metrics":      map[string]any{},
				},
			},
		})
		if !beforeHookCheck(input) {
			t.Fatalf("expected true when module enabled and metrics map present")
		}

		var found bool
		for _, op := range pv.GetPatches() {
			if op.Op != "add" && op.Op != "replace" {
				continue
			}
			if op.Path != patchPath(settings.InternalMetricsCertPath) {
				continue
			}
			found = true
			v := decodePatchValue(t, op)
			m, ok := v.(map[string]any)
			if !ok || len(m) != 0 {
				t.Fatalf("expected empty cert object patch, got %#v", v)
			}
		}
		if !found {
			t.Fatalf("expected cert object patch, patches: %#v", pv.GetPatches())
		}
	})

	t.Run("keeps existing cert object", func(t *testing.T) {
		input, pv := newHookInput(t, map[string]any{
			settings.ConfigRoot: map[string]any{
				"internal": map[string]any{
					"moduleConfig": map[string]any{"enabled": true},
					"metrics": map[string]any{
						"cert": map[string]any{"tls.crt": "data"},
					},
				},
			},
		})
		if !beforeHookCheck(input) {
			t.Fatalf("expected true when cert object already present")
		}

		for _, op := range pv.GetPatches() {
			if op.Path == patchPath(settings.InternalMetricsCertPath) {
				t.Fatalf("expected no cert patch when cert exists, got %#v", op)
			}
		}
	})
}
