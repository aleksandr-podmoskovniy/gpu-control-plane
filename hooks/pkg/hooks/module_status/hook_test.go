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

package module_status

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	pkg "github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/hooks/pkg/settings"
)

func newHookInput(t *testing.T, values map[string]any) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	t.Helper()

	patchable, err := patchablevalues.NewPatchableValues(values)
	if err != nil {
		t.Fatalf("create patchable values: %v", err)
	}

	input := &pkg.HookInput{
		Values: patchable,
	}

	return input, patchable
}

func decodePatchValue(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	if len(raw) == 0 {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode patch value: %v", err)
	}
	return out
}

func slashPath(dotPath string) string {
	return "/" + strings.ReplaceAll(dotPath, ".", "/")
}

func TestHandleModuleStatusRemovesStateWhenModuleDisabled(t *testing.T) {
	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"moduleConfig": map[string]any{"enabled": false},
				"conditions":   []any{map[string]any{"type": conditionTypePrereq}},
				"moduleConfigValidation": map[string]any{
					"error":  "previous",
					"source": validationSource,
				},
			},
		},
	}

	input, patchable := newHookInput(t, values)

	if err := handleModuleStatus(context.Background(), input); err != nil {
		t.Fatalf("handleModuleStatus returned error: %v", err)
	}

	patches := patchable.GetPatches()
	if len(patches) != 2 {
		t.Fatalf("expected 2 patches, got %d", len(patches))
	}

	if patches[0].Op != "remove" || patches[0].Path != slashPath(settings.InternalModuleConditionsPath) {
		t.Fatalf("unexpected first patch: %+v", patches[0])
	}
	if patches[1].Op != "remove" || patches[1].Path != slashPath(settings.InternalModuleValidationPath) {
		t.Fatalf("unexpected second patch: %+v", patches[1])
	}
}

func TestHandleModuleStatusAddsConditionWhenNfdMissing(t *testing.T) {
	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"moduleConfig": map[string]any{"enabled": true},
			},
		},
		"global": map[string]any{
			"enabledModules": []any{"cert-manager"},
		},
	}

	input, patchable := newHookInput(t, values)

	if err := handleModuleStatus(context.Background(), input); err != nil {
		t.Fatalf("handleModuleStatus returned error: %v", err)
	}

	patches := patchable.GetPatches()
	if len(patches) != 2 {
		t.Fatalf("expected 2 patches, got %d", len(patches))
	}

	condPatch := patches[0]
	if condPatch.Op != "add" || condPatch.Path != slashPath(settings.InternalModuleConditionsPath) {
		t.Fatalf("unexpected conditions patch: %+v", condPatch)
	}
	conditions := decodePatchValue(t, condPatch.Value)
	list, ok := conditions.([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("unexpected conditions payload: %#v", conditions)
	}
	item, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected condition entry type: %#v", list[0])
	}
	if item["type"] != conditionTypePrereq || item["reason"] != reasonNFDDisabled {
		t.Fatalf("unexpected condition entry: %#v", item)
	}

	validationPatch := patches[1]
	if validationPatch.Op != "add" || validationPatch.Path != slashPath(settings.InternalModuleValidationPath) {
		t.Fatalf("unexpected validation patch: %+v", validationPatch)
	}
	payload, ok := decodePatchValue(t, validationPatch.Value).(map[string]any)
	if !ok {
		t.Fatalf("unexpected validation payload type: %#v", validationPatch.Value)
	}
	if payload["source"] != validationSource {
		t.Fatalf("unexpected validation source: %#v", payload)
	}
	if payload["error"] == "" {
		t.Fatalf("validation error message is empty: %#v", payload)
	}
}

func TestHandleModuleStatusClearsStateWhenPrereqSatisfied(t *testing.T) {
	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"moduleConfig": map[string]any{"enabled": true},
				"conditions": []any{map[string]any{
					"type":    conditionTypePrereq,
					"reason":  reasonNFDDisabled,
					"message": "Module gpu-control-plane requires the node-feature-discovery module to be enabled",
				}},
				"moduleConfigValidation": map[string]any{
					"error":  "Module gpu-control-plane requires the node-feature-discovery module to be enabled",
					"source": validationSource,
				},
			},
		},
		"global": map[string]any{
			"enabledModules": []any{"node-feature-discovery"},
		},
	}

	input, patchable := newHookInput(t, values)

	if err := handleModuleStatus(context.Background(), input); err != nil {
		t.Fatalf("handleModuleStatus returned error: %v", err)
	}

	patches := patchable.GetPatches()
	if len(patches) != 2 {
		t.Fatalf("expected 2 patches, got %d", len(patches))
	}

	if patches[0].Op != "remove" || patches[0].Path != slashPath(settings.InternalModuleConditionsPath) {
		t.Fatalf("unexpected conditions removal patch: %+v", patches[0])
	}
	if patches[1].Op != "remove" || patches[1].Path != slashPath(settings.InternalModuleValidationPath) {
		t.Fatalf("unexpected validation removal patch: %+v", patches[1])
	}
}
