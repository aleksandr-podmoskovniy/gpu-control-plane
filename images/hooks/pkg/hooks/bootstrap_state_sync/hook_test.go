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
	if snapshot, ok := s.value.(physicalSnapshot); ok {
		if out, ok := target.(*physicalSnapshot); ok {
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

func TestHandleBootstrapStateSyncKeepsStateWhenConditionsEmpty(t *testing.T) {
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
	if patch == nil || patch.Op != "add" {
		t.Fatalf("expected add patch for %s, got %#v", settings.InternalBootstrapStatePath, patch)
	}
}

func TestHandleBootstrapStateSyncSkipsInvalidSnapshot(t *testing.T) {
	input, patchable := newHookInput(t, map[string]any{}, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {
			jsonSnapshot{err: errors.New("boom")},
			jsonSnapshot{value: inventorySnapshot{
				Name: "node-valid",
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

func TestHandleBootstrapStateSyncPopulatesValues(t *testing.T) {
	input, patchable := newHookInput(t, map[string]any{}, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {
			jsonSnapshot{value: inventorySnapshot{
				Name: "node-a",
				Status: snapshotStatus{Conditions: []metav1.Condition{
					{Type: "DriverReady", Status: metav1.ConditionTrue},
					{Type: "ToolkitReady", Status: metav1.ConditionTrue},
				}},
			}},
			jsonSnapshot{value: inventorySnapshot{
				Name: "node-b",
				Status: snapshotStatus{Conditions: []metav1.Condition{
					{Type: "DriverReady", Status: metav1.ConditionFalse},
				}},
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
	if nodeA["driverReady"] != true || nodeA["toolkitReady"] != true {
		t.Fatalf("unexpected node-a payload: %#v", nodeA)
	}

	nodeB := nodes["node-b"].(map[string]any)
	if nodeB["driverReady"] != false {
		t.Fatalf("unexpected node-b payload: %#v", nodeB)
	}

	components := payloadValue["components"].(map[string]any)
	expectNodes := map[string][]string{
		settings.BootstrapComponentValidator:           {"node-a", "node-b"},
		settings.BootstrapComponentGPUFeatureDiscovery: {"node-a"},
		settings.BootstrapComponentDCGM:                {"node-a"},
		settings.BootstrapComponentDCGMExporter:        {"node-a"},
		"handler":                                      {},
	}
	for component, nodesList := range expectNodes {
		entry := components[component].(map[string]any)
		nodesVal, ok := entry["nodes"]
		if !ok {
			t.Fatalf("component %s missing nodes field", component)
		}
		actual := nodesVal.([]any)
		if len(actual) != len(nodesList) {
			t.Fatalf("component %s nodes mismatch: %#v", component, actual)
		}
		for i, node := range nodesList {
			if actual[i].(string) != node {
				t.Fatalf("component %s node[%d]=%s, expected %s", component, i, actual[i], node)
			}
		}
		hash := entry["hash"].(string)
		if hash == "" {
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
		"handler",
	} {
		entry, ok := components[component]
		if !ok {
			t.Fatalf("component %s missing in values: %#v", component, components)
		}
		data := entry.(map[string]any)
		nodes := data["nodes"].([]any)
		if component == settings.BootstrapComponentValidator {
			if len(nodes) != 1 || nodes[0].(string) != "node-a" {
				t.Fatalf("validator nodes expected [node-a], got %#v", nodes)
			}
			if hash := data["hash"].(string); hash != hashStrings([]string{"node-a"}) {
				t.Fatalf("validator hash expected %s, got %s", hashStrings([]string{"node-a"}), hash)
			}
			continue
		}
		if len(nodes) != 0 {
			t.Fatalf("component %s nodes expected empty, got %#v", component, nodes)
		}
		if hash := data["hash"].(string); hash != hashStrings(nil) {
			t.Fatalf("component %s hash expected %s for empty nodes, got %s", component, hashStrings(nil), hash)
		}
	}
}

func TestHandleBootstrapStateSyncPrefersPhysicalGPUForValidator(t *testing.T) {
	input, patchable := newHookInput(t, map[string]any{}, map[string][]pkg.Snapshot{
		bootstrapStateSnapshot: {
			jsonSnapshot{value: inventorySnapshot{
				Name: "node-a",
				Status: snapshotStatus{Conditions: []metav1.Condition{
					{Type: "DriverReady", Status: metav1.ConditionTrue},
					{Type: "ToolkitReady", Status: metav1.ConditionTrue},
				}},
			}},
			jsonSnapshot{value: inventorySnapshot{
				Name: "node-b",
				Status: snapshotStatus{Conditions: []metav1.Condition{
					{Type: "DriverReady", Status: metav1.ConditionFalse},
				}},
			}},
		},
		physicalGPUSnapshot: {
			jsonSnapshot{value: physicalSnapshot{NodeName: "node-b", VendorID: "10de"}},
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

	validator := components[settings.BootstrapComponentValidator].(map[string]any)
	validatorNodes := validator["nodes"].([]any)
	if len(validatorNodes) != 1 || validatorNodes[0].(string) != "node-b" {
		t.Fatalf("validator nodes expected [node-b], got %#v", validatorNodes)
	}

	handler := components["handler"].(map[string]any)
	handlerNodes := handler["nodes"].([]any)
	if len(handlerNodes) != 1 || handlerNodes[0].(string) != "node-b" {
		t.Fatalf("handler nodes expected [node-b], got %#v", handlerNodes)
	}

	dcgm := components[settings.BootstrapComponentDCGM].(map[string]any)
	dcgmNodes := dcgm["nodes"].([]any)
	if len(dcgmNodes) != 1 || dcgmNodes[0].(string) != "node-b" {
		t.Fatalf("dcgm nodes expected [node-b], got %#v", dcgmNodes)
	}

	dcgmExporter := components[settings.BootstrapComponentDCGMExporter].(map[string]any)
	dcgmExporterNodes := dcgmExporter["nodes"].([]any)
	if len(dcgmExporterNodes) != 1 || dcgmExporterNodes[0].(string) != "node-b" {
		t.Fatalf("dcgm-exporter nodes expected [node-b], got %#v", dcgmExporterNodes)
	}

	gfd := components[settings.BootstrapComponentGPUFeatureDiscovery].(map[string]any)
	gfdNodes := gfd["nodes"].([]any)
	if len(gfdNodes) != 1 || gfdNodes[0].(string) != "node-a" {
		t.Fatalf("gfd nodes expected [node-a], got %#v", gfdNodes)
	}
}
