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

package bootstrap_state_sync

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse/pkg/log"
	pkg "github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"
	"github.com/deckhouse/module-sdk/pkg/utils"

	"hooks/pkg/settings"
)

type snapshotStore struct {
	items map[string][]pkg.Snapshot
}

func (s snapshotStore) Get(key string) []pkg.Snapshot {
	return s.items[key]
}

type jsonSnapshot struct {
	value any
	err   error
}

func (s jsonSnapshot) UnmarshalTo(target any) error {
	if s.err != nil {
		return s.err
	}
	raw, err := json.Marshal(s.value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func (s jsonSnapshot) String() string {
	raw, _ := json.Marshal(s.value)
	return string(raw)
}

func newHookInput(t *testing.T, values map[string]any, snapshots map[string][]pkg.Snapshot) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	t.Helper()

	patchable, err := patchablevalues.NewPatchableValues(values)
	if err != nil {
		t.Fatalf("create patchable values: %v", err)
	}

	if snapshots == nil {
		snapshots = map[string][]pkg.Snapshot{}
	}

	return &pkg.HookInput{
		Values:    patchable,
		Snapshots: snapshotStore{items: snapshots},
		Logger:    log.NewNop(),
	}, patchable
}

func pathToJSONPointer(path string) string {
	return "/" + strings.ReplaceAll(path, ".", "/")
}

func decodePatchValue(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	var out any
	if len(raw) == 0 {
		return out
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode patch value: %v", err)
	}
	return out
}

func lastPatch(patches []*utils.ValuesPatchOperation, path string) *utils.ValuesPatchOperation {
	for i := len(patches) - 1; i >= 0; i-- {
		if patches[i].Path == path {
			return patches[i]
		}
	}
	return nil
}

func TestHandleBootstrapStateSyncRemovesStateWhenSnapshotMissing(t *testing.T) {
	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"bootstrap": map[string]any{"version": "stale"},
			},
		},
	}

	input, patchable := newHookInput(t, values, nil)

	if err := handleBootstrapStateSync(context.Background(), input); err != nil {
		t.Fatalf("handleBootstrapStateSync returned error: %v", err)
	}

	patch := lastPatch(patchable.GetPatches(), pathToJSONPointer(settings.InternalBootstrapStatePath))
	if patch == nil || patch.Op != "remove" {
		t.Fatalf("expected remove patch for %s, got %#v", settings.InternalBootstrapStatePath, patch)
	}
}

func TestHandleBootstrapStateSyncRemovesStateWhenConfigMapEmpty(t *testing.T) {
	values := map[string]any{}

	input, patchable := newHookInput(t, values, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {
			jsonSnapshot{value: configMapSnapshot{Data: map[string]string{}}},
		},
	})

	if err := handleBootstrapStateSync(context.Background(), input); err != nil {
		t.Fatalf("handleBootstrapStateSync returned error: %v", err)
	}

	if patch := lastPatch(patchable.GetPatches(), pathToJSONPointer(settings.InternalBootstrapStatePath)); patch != nil {
		t.Fatalf("expected no patch for %s when ConfigMap is empty, got %#v", settings.InternalBootstrapStatePath, patch)
	}
}

func TestHandleBootstrapStateSyncPopulatesValues(t *testing.T) {
	payload := configMapSnapshot{
		Data: map[string]string{
			"node-a.yaml": `
phase: Ready
components:
  validator: true
  gpu-feature-discovery: true
  dcgm: true
  dcgm-exporter: true
updatedAt: "2025-01-01T00:00:00Z"
`,
			"node-b.yaml": `
phase: Validating
components:
  validator: true
`,
		},
	}

	input, patchable := newHookInput(t, map[string]any{}, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {jsonSnapshot{value: payload}},
	})

	if err := handleBootstrapStateSync(context.Background(), input); err != nil {
		t.Fatalf("handleBootstrapStateSync returned error: %v", err)
	}

	patches := patchable.GetPatches()
	if len(patches) != 1 {
		t.Fatalf("expected single patch, got %d", len(patches))
	}

	patch := patches[0]
	if patch.Op != "add" || patch.Path != pathToJSONPointer(settings.InternalBootstrapStatePath) {
		t.Fatalf("unexpected patch: %#v", patch)
	}

	payloadValue := decodePatchValue(t, patch.Value).(map[string]any)

	nodes := payloadValue["nodes"].(map[string]any)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	nodeA := nodes["node-a"].(map[string]any)
	if nodeA["phase"] != "Ready" {
		t.Fatalf("unexpected node-a payload: %#v", nodeA)
	}
	compA := nodeA["components"].(map[string]any)
	for _, key := range []string{settings.BootstrapComponentValidator, settings.BootstrapComponentGPUFeatureDiscovery, settings.BootstrapComponentDCGM, settings.BootstrapComponentDCGMExporter} {
		if val, ok := compA[key]; !ok || val != true {
			t.Fatalf("component %s missing for node-a: %#v", key, compA)
		}
	}
	if nodeA["updatedAt"] == "" {
		t.Fatalf("expected updatedAt for node-a")
	}

	nodeB := nodes["node-b"].(map[string]any)
	if nodeB["phase"] != "Validating" {
		t.Fatalf("unexpected node-b payload: %#v", nodeB)
	}
	compB := nodeB["components"].(map[string]any)
	if len(compB) != 1 || compB[settings.BootstrapComponentValidator] != true {
		t.Fatalf("unexpected node-b components: %#v", compB)
	}

	components := payloadValue["components"].(map[string]any)
	expectNodes := map[string][]string{
		settings.BootstrapComponentValidator:           {"node-a", "node-b"},
		settings.BootstrapComponentGPUFeatureDiscovery: {"node-a"},
		settings.BootstrapComponentDCGM:                {"node-a"},
		settings.BootstrapComponentDCGMExporter:        {"node-a"},
	}
	for component, nodesList := range expectNodes {
		entry := components[component].(map[string]any)
		nodesVal, ok := entry["nodes"]
		if !ok {
			t.Fatalf("component %s missing nodes field", component)
		}
		if nodesVal == nil {
			if len(nodesList) != 0 {
				t.Fatalf("component %s expected nodes %v, got nil", component, nodesList)
			}
		} else {
			actual := nodesVal.([]any)
			if len(actual) != len(nodesList) {
				t.Fatalf("component %s nodes mismatch: %#v", component, actual)
			}
			for i, node := range nodesList {
				if actual[i].(string) != node {
					t.Fatalf("component %s node[%d]=%s, expected %s", component, i, actual[i], node)
				}
			}
		}
		hash := entry["hash"].(string)
		if len(nodesList) == 0 {
			if hash != "" {
				t.Fatalf("component %s hash expected empty, got %s", component, hash)
			}
		} else if hash == "" {
			t.Fatalf("component %s hash is empty", component)
		}
	}

	if payloadValue["version"].(string) == "" {
		t.Fatalf("expected non-empty version hash")
	}
}

func TestHandleBootstrapStateSyncIncludesEmptyComponents(t *testing.T) {
	payload := configMapSnapshot{
		Data: map[string]string{
			"node-a.yaml": `
phase: Disabled
components: {}
`,
		},
	}

	input, patchable := newHookInput(t, map[string]any{}, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {jsonSnapshot{value: payload}},
	})

	if err := handleBootstrapStateSync(context.Background(), input); err != nil {
		t.Fatalf("handleBootstrapStateSync returned error: %v", err)
	}

	patches := patchable.GetPatches()
	if len(patches) != 1 {
		t.Fatalf("expected single patch, got %d", len(patches))
	}

	payloadValue := decodePatchValue(t, patches[0].Value).(map[string]any)
	components := payloadValue["components"].(map[string]any)
	for _, component := range []string{
		settings.BootstrapComponentValidator,
		settings.BootstrapComponentGPUFeatureDiscovery,
		settings.BootstrapComponentDCGM,
		settings.BootstrapComponentDCGMExporter,
	} {
		entry, ok := components[component]
		if !ok {
			t.Fatalf("component %s missing in values: %#v", component, components)
		}
		data := entry.(map[string]any)
		if nodesVal, exists := data["nodes"]; exists && nodesVal != nil {
			nodes := nodesVal.([]any)
			if len(nodes) != 0 {
				t.Fatalf("component %s nodes expected empty, got %#v", component, nodes)
			}
		}
		if hash := data["hash"].(string); hash != hashStrings(nil) {
			t.Fatalf("component %s hash expected %s for empty nodes, got %s", component, hashStrings(nil), hash)
		}
	}
}
