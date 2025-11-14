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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deckhouse/deckhouse/pkg/log"
	pkg "github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"
	"github.com/deckhouse/module-sdk/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	if snapshot, ok := s.value.(inventorySnapshot); ok {
		if out, ok := target.(*inventorySnapshot); ok {
			*out = snapshot
			return nil
		}
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

func TestHandleBootstrapStateSyncRemovesStateWhenInventoriesEmpty(t *testing.T) {
	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"bootstrap": map[string]any{"version": "stale"},
			},
		},
	}

	input, patchable := newHookInput(t, values, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {
			jsonSnapshot{value: inventorySnapshot{Name: "node-a"}},
		},
	})

	if err := handleBootstrapStateSync(context.Background(), input); err != nil {
		t.Fatalf("handleBootstrapStateSync returned error: %v", err)
	}

	patch := lastPatch(patchable.GetPatches(), pathToJSONPointer(settings.InternalBootstrapStatePath))
	if patch == nil || patch.Op != "remove" {
		t.Fatalf("expected remove patch for %s, got %#v", settings.InternalBootstrapStatePath, patch)
	}
}

func TestHandleBootstrapStateSyncSkipsInvalidSnapshot(t *testing.T) {
	input, patchable := newHookInput(t, map[string]any{}, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {
			jsonSnapshot{err: errors.New("boom")},
			jsonSnapshot{value: inventorySnapshot{
				Name: "node-valid",
				Status: inventorySnapshotStatus{
					Bootstrap: inventoryBootstrapStatus{
						Phase:      "Ready",
						Components: map[string]bool{settings.BootstrapComponentValidator: true},
					},
				},
			}},
		},
	})

	if err := handleBootstrapStateSync(context.Background(), input); err != nil {
		t.Fatalf("handleBootstrapStateSync returned error: %v", err)
	}

	patches := patchable.GetPatches()
	if len(patches) != 1 {
		t.Fatalf("expected single patch, got %d", len(patches))
	}
	nodes := decodePatchValue(t, patches[0].Value).(map[string]any)["nodes"].(map[string]any)
	if len(nodes) != 1 || nodes["node-valid"] == nil {
		t.Fatalf("expected only valid node in state: %#v", nodes)
	}
}

func TestHandleBootstrapStateSyncSkipsBlankNodes(t *testing.T) {
	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"bootstrap": map[string]any{"version": "old"},
			},
		},
	}
	input, patchable := newHookInput(t, values, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {
			jsonSnapshot{value: inventorySnapshot{Name: "   "}},
		},
	})

	if err := handleBootstrapStateSync(context.Background(), input); err != nil {
		t.Fatalf("handleBootstrapStateSync returned error: %v", err)
	}

	patch := lastPatch(patchable.GetPatches(), pathToJSONPointer(settings.InternalBootstrapStatePath))
	if patch == nil || patch.Op != "remove" {
		t.Fatalf("expected removal patch for blank node snapshot, got %#v", patch)
	}
}

func TestHandleBootstrapStateSyncSkipsEmptyEntries(t *testing.T) {
	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"bootstrap": map[string]any{"version": "old"},
			},
		},
	}
	input, patchable := newHookInput(t, values, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {
			jsonSnapshot{value: inventorySnapshot{
				Name: "node-empty",
				Status: inventorySnapshotStatus{
					Bootstrap: inventoryBootstrapStatus{
						LastRun: &metav1.Time{}, // non-nil but zero to exercise len(entry)==0 branch
					},
				},
			}},
		},
	})

	if err := handleBootstrapStateSync(context.Background(), input); err != nil {
		t.Fatalf("handleBootstrapStateSync returned error: %v", err)
	}

	patch := lastPatch(patchable.GetPatches(), pathToJSONPointer(settings.InternalBootstrapStatePath))
	if patch == nil || patch.Op != "remove" {
		t.Fatalf("expected removal patch for empty entry, got %#v", patch)
	}
}

func TestHandleBootstrapStateSyncPopulatesValues(t *testing.T) {
	ts := metav1.NewTime(time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC))
	input, patchable := newHookInput(t, map[string]any{}, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {
			jsonSnapshot{value: inventorySnapshot{
				Name: "node-a",
				Status: inventorySnapshotStatus{
					Bootstrap: inventoryBootstrapStatus{
						Phase: "Ready",
						Components: map[string]bool{
							settings.BootstrapComponentValidator:           true,
							settings.BootstrapComponentGPUFeatureDiscovery: true,
							settings.BootstrapComponentDCGM:                true,
							settings.BootstrapComponentDCGMExporter:        true,
						},
						LastRun:        &ts,
						PendingDevices: []string{"gpu-a"},
					},
				},
			}},
			jsonSnapshot{value: inventorySnapshot{
				Name: "node-b",
				Status: inventorySnapshotStatus{
					Bootstrap: inventoryBootstrapStatus{
						Phase:             "Validating",
						Components:        map[string]bool{settings.BootstrapComponentValidator: true},
						ValidatorRequired: true,
					},
				},
			}},
		},
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
	if pending := nodeA["pendingDevices"].([]any); len(pending) != 1 || pending[0].(string) != "gpu-a" {
		t.Fatalf("unexpected pendingDevices for node-a: %#v", pending)
	}

	nodeB := nodes["node-b"].(map[string]any)
	if nodeB["phase"] != "Validating" {
		t.Fatalf("unexpected node-b payload: %#v", nodeB)
	}
	if nodeB["validatorRequired"] != true {
		t.Fatalf("expected validatorRequired for node-b, got %#v", nodeB)
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
	input, patchable := newHookInput(t, map[string]any{}, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {
			jsonSnapshot{value: inventorySnapshot{
				Name: "node-a",
				Status: inventorySnapshotStatus{
					Bootstrap: inventoryBootstrapStatus{
						Phase:      "Disabled",
						Components: map[string]bool{},
					},
				},
			}},
		},
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

func TestHandleBootstrapStateSyncHandlesUnknownComponents(t *testing.T) {
	input, patchable := newHookInput(t, map[string]any{}, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {
			jsonSnapshot{value: inventorySnapshot{
				Name: "node-x",
				Status: inventorySnapshotStatus{
					Bootstrap: inventoryBootstrapStatus{
						Phase: "Ready",
						Components: map[string]bool{
							settings.BootstrapComponentValidator: false,
							"custom-component":                   true,
						},
					},
				},
			}},
		},
	})

	if err := handleBootstrapStateSync(context.Background(), input); err != nil {
		t.Fatalf("handleBootstrapStateSync returned error: %v", err)
	}

	patch := lastPatch(patchable.GetPatches(), pathToJSONPointer(settings.InternalBootstrapStatePath))
	if patch == nil {
		t.Fatalf("expected bootstrap state patch")
	}
	payload := decodePatchValue(t, patch.Value).(map[string]any)
	components := payload["components"].(map[string]any)

	custom, ok := components["custom-component"]
	if !ok {
		t.Fatalf("custom component missing in components map: %#v", components)
	}
	nodes := custom.(map[string]any)["nodes"].([]any)
	if len(nodes) != 1 || nodes[0].(string) != "node-x" {
		t.Fatalf("expected custom component bound to node-x, got %#v", nodes)
	}

	validator := components[settings.BootstrapComponentValidator].(map[string]any)
	if nodesVal := validator["nodes"]; nodesVal != nil && len(nodesVal.([]any)) != 0 {
		t.Fatalf("validator nodes should stay empty when disabled: %#v", nodesVal)
	}
}

func TestHandleBootstrapStateSyncCopiesValidatorFields(t *testing.T) {
	components := map[string]bool{
		settings.BootstrapComponentValidator:           true,
		settings.BootstrapComponentGPUFeatureDiscovery: true,
	}
	input, patchable := newHookInput(t, map[string]any{}, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {
			jsonSnapshot{value: inventorySnapshot{
				Name: "node-c",
				Status: inventorySnapshotStatus{
					Bootstrap: inventoryBootstrapStatus{
						Phase:             "Monitoring",
						Components:        components,
						ValidatorRequired: true,
						PendingDevices:    []string{"gpu-0", "gpu-1"},
					},
				},
			}},
		},
	})

	if err := handleBootstrapStateSync(context.Background(), input); err != nil {
		t.Fatalf("handleBootstrapStateSync returned error: %v", err)
	}

	patch := lastPatch(patchable.GetPatches(), pathToJSONPointer(settings.InternalBootstrapStatePath))
	if patch == nil {
		t.Fatalf("expected bootstrap state patch")
	}
	payload := decodePatchValue(t, patch.Value).(map[string]any)
	nodes := payload["nodes"].(map[string]any)
	node := nodes["node-c"].(map[string]any)
	if node["validatorRequired"] != true {
		t.Fatalf("expected validatorRequired flag, got %v", node["validatorRequired"])
	}
	pending := node["pendingDevices"].([]any)
	if len(pending) != 2 {
		t.Fatalf("expected pending devices propagated, got %v", pending)
	}

	components[settings.BootstrapComponentValidator] = false
	compMap := node["components"].(map[string]any)
	if compMap[settings.BootstrapComponentValidator] != true {
		t.Fatalf("components map should be copied, got %v", compMap)
	}
}

func TestCopyComponentsReturnsNil(t *testing.T) {
	if copyComponents(map[string]bool{}) != nil {
		t.Fatalf("expected nil copy for empty map")
	}
}
