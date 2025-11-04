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

package discovery_node_feature_rule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	pkg "github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"
	"github.com/deckhouse/module-sdk/pkg/utils"
	"github.com/deckhouse/module-sdk/testing/mock"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	"hooks/pkg/settings"
)

func newHookInput(t *testing.T, snapshots pkg.Snapshots, pc pkg.OutputPatchCollector, values map[string]any) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	t.Helper()

	patchable, err := patchablevalues.NewPatchableValues(values)
	if err != nil {
		t.Fatalf("create patchable values: %v", err)
	}

	return &pkg.HookInput{
		Values:         patchable,
		Snapshots:      snapshots,
		PatchCollector: pc,
	}, patchable
}

func resetSeams() {
	namespaceEnsurer = ensureNamespace
	nodeFeatureEnsurer = ensureNodeFeatureRule
	yamlUnmarshal = defaultYAMLUnmarshal
}

func TestHandleNodeFeatureRuleSyncModuleDisabled(t *testing.T) {
	resetSeams()
	requireNFDModule = false
	t.Cleanup(func() { requireNFDModule = true })

	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(key string) []pkg.Snapshot {
		if key != moduleConfigSnapshot {
			t.Fatalf("unexpected snapshot key: %s", key)
		}
		return nil
	})

	pc := mock.NewPatchCollectorMock(t)
	var deletes []string
	pc.DeleteMock.Set(func(apiVersion, kind, namespace, name string) {
		deletes = append(deletes, fmt.Sprintf("%s/%s/%s/%s", apiVersion, kind, namespace, name))
	})

	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"nodeFeatureRule": map[string]any{
					"name": "old",
				},
			},
		},
	}

	input, patchable := newHookInput(t, snapshots, pc, values)

	if err := handleNodeFeatureRuleSync(context.Background(), input); err != nil {
		t.Fatalf("handleNodeFeatureRuleSync returned error: %v", err)
	}

	expected := map[string]struct{}{
		"nfd.k8s-sigs.io/v1alpha1/NodeFeatureRule//" + settings.NodeFeatureRuleName: {},
		"v1/Namespace//" + settings.ModuleNamespace:                                 {},
	}
	for _, delete := range deletes {
		if _, ok := expected[delete]; !ok {
			t.Fatalf("unexpected delete operation: %s", delete)
		}
	}

	removed := false
	for _, patch := range patchable.GetPatches() {
		if patch.Op == "remove" && patch.Path == expectedPatchPath(settings.InternalNodeFeatureRulePath) {
			removed = true
		}
	}
	if !removed {
		t.Fatalf("expected removal of %s, patches: %#v", settings.InternalNodeFeatureRulePath, patchable.GetPatches())
	}
}

func TestHandleNodeFeatureRuleSyncDependencyMissing(t *testing.T) {
	resetSeams()
	requireNFDModule = true
	t.Cleanup(func() { requireNFDModule = true })

	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Optional()

	pc := mock.NewPatchCollectorMock(t)
	var deletes []string
	pc.DeleteMock.Set(func(apiVersion, kind, namespace, name string) {
		deletes = append(deletes, fmt.Sprintf("%s/%s/%s/%s", apiVersion, kind, namespace, name))
	})

	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"nodeFeatureRule": map[string]any{"name": "stale"},
			},
		},
	}

	input, patchable := newHookInput(t, snapshots, pc, values)

	err := handleNodeFeatureRuleSync(context.Background(), input)
	if err == nil || err.Error() != settings.NFDDependencyErrorMessage {
		t.Fatalf("expected dependency error, got: %v", err)
	}

	expectedDeletes := map[string]struct{}{
		"nfd.k8s-sigs.io/v1alpha1/NodeFeatureRule//" + settings.NodeFeatureRuleName: {},
		"v1/Namespace//" + settings.ModuleNamespace:                                 {},
	}
	for _, del := range deletes {
		if _, ok := expectedDeletes[del]; !ok {
			t.Fatalf("unexpected delete: %s", del)
		}
	}

	var removed bool
	for _, patch := range patchable.GetPatches() {
		if patch.Op == "remove" && patch.Path == expectedPatchPath(settings.InternalNodeFeatureRulePath) {
			removed = true
		}
	}
	if !removed {
		t.Fatalf("expected removal patch for %s", settings.InternalNodeFeatureRulePath)
	}
}

func TestHandleNodeFeatureRuleSyncCreatesResources(t *testing.T) {
	resetSeams()
	requireNFDModule = false
	t.Cleanup(func() { requireNFDModule = true })

	enabled := true
	payload := moduleConfigSnapshotPayload{
		Spec: moduleConfigSnapshotSpec{
			Enabled: ptr.To(enabled),
		},
	}
	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(target any) error {
		return json.Unmarshal(mustMarshal(payload), target)
	})

	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(key string) []pkg.Snapshot {
		if key != moduleConfigSnapshot {
			t.Fatalf("unexpected snapshot key: %s", key)
		}
		return []pkg.Snapshot{snapshot}
	})

	pc := mock.NewPatchCollectorMock(t)
	var created []any
	pc.CreateIfNotExistsMock.Set(func(obj any) { created = append(created, obj) })
	pc.CreateOrUpdateMock.Set(func(obj any) { created = append(created, obj) })

	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"nodeFeatureRule": map[string]any{},
			},
		},
		"global": map[string]any{
			"enabledModules": []any{"node-feature-discovery"},
		},
	}

	input, patchable := newHookInput(t, snapshots, pc, values)

	if err := handleNodeFeatureRuleSync(context.Background(), input); err != nil {
		t.Fatalf("handleNodeFeatureRuleSync returned error: %v", err)
	}

	if len(created) != 2 {
		t.Fatalf("expected namespace and NodeFeatureRule creations, got %d", len(created))
	}

	foundNF := false
	for _, obj := range created {
		if unstr, ok := obj.(*unstructured.Unstructured); ok && unstr.GetKind() == "NodeFeatureRule" {
			if unstr.GetName() != settings.NodeFeatureRuleName {
				t.Fatalf("unexpected NodeFeatureRule name: %s", unstr.GetName())
			}
			labels := unstr.GetLabels()
			if labels["app.kubernetes.io/name"] != settings.ModuleName {
				t.Fatalf("module label missing: %v", labels)
			}
			foundNF = true
		}
	}
	if !foundNF {
		t.Fatal("NodeFeatureRule was not created")
	}

	var set bool
	for _, patch := range patchable.GetPatches() {
		if (patch.Op == "replace" || patch.Op == "add") && patch.Path == expectedPatchPath(settings.InternalNodeFeatureRulePath) {
			set = true
		}
	}
	if !set {
		t.Fatalf("expected patch for %s", settings.InternalNodeFeatureRulePath)
	}
}

func TestModuleConfigEnabledDecodingError(t *testing.T) {
	resetSeams()
	requireNFDModule = false
	t.Cleanup(func() { requireNFDModule = true })

	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(any) error {
		return errors.New("boom")
	})

	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(key string) []pkg.Snapshot {
		if key != moduleConfigSnapshot {
			t.Fatalf("unexpected snapshot key: %s", key)
		}
		return []pkg.Snapshot{snapshot}
	})

	input, _ := newHookInput(t, snapshots, mock.NewPatchCollectorMock(t), map[string]any{})

	if err := handleNodeFeatureRuleSync(context.Background(), input); err == nil || !strings.Contains(err.Error(), "decode ModuleConfig") {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

func TestModuleConfigEnabledNil(t *testing.T) {
	resetSeams()
	requireNFDModule = false
	t.Cleanup(func() { requireNFDModule = true })

	payload := moduleConfigSnapshotPayload{}
	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(target any) error {
		return json.Unmarshal(mustMarshal(payload), target)
	})

	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(key string) []pkg.Snapshot {
		if key != moduleConfigSnapshot {
			t.Fatalf("unexpected snapshot key: %s", key)
		}
		return []pkg.Snapshot{snapshot}
	})

	pc := mock.NewPatchCollectorMock(t)
	pc.DeleteMock.Set(func(string, string, string, string) {})

	input, patchable := newHookInput(t, snapshots, pc, map[string]any{})

	if err := handleNodeFeatureRuleSync(context.Background(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pc.CreateIfNotExistsBeforeCounter() != 0 {
		t.Fatalf("expected no namespace creation when enabled not set")
	}
	for _, patch := range patchable.GetPatches() {
		if patch.Path == expectedPatchPath(settings.InternalNodeFeatureRulePath) {
			t.Fatalf("did not expect nodeFeatureRule patch, got %+v", patch)
		}
	}
}

func TestHandleNodeFeatureRuleSyncNamespaceError(t *testing.T) {
	resetSeams()
	requireNFDModule = true
	t.Cleanup(func() { requireNFDModule = true })

	enabled := true
	payload := moduleConfigSnapshotPayload{
		Spec: moduleConfigSnapshotSpec{
			Enabled: ptr.To(enabled),
		},
	}
	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(target any) error {
		return json.Unmarshal(mustMarshal(payload), target)
	})

	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(key string) []pkg.Snapshot {
		if key != moduleConfigSnapshot {
			t.Fatalf("unexpected snapshot key: %s", key)
		}
		return []pkg.Snapshot{snapshot}
	})

	namespaceEnsurer = func(pkg.PatchCollector) error {
		return errors.New("namespace error")
	}

	values := map[string]any{
		"global": map[string]any{
			"enabledModules": []any{"node-feature-discovery"},
		},
	}

	pc := mock.NewPatchCollectorMock(t)
	pc.CreateIfNotExistsMock.Optional()

	input, patchable := newHookInput(t, snapshots, pc, values)

	if err := handleNodeFeatureRuleSync(context.Background(), input); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	patch := findPatchByPath(t, patchable, settings.InternalNodeFeatureRulePath)
	status := decodePatchMap(t, patch.Value)
	if !strings.Contains(status["error"].(string), "namespace error") {
		t.Fatalf("unexpected error payload: %#v", status)
	}
}

func TestHandleNodeFeatureRuleSyncRuleError(t *testing.T) {
	resetSeams()
	requireNFDModule = true
	t.Cleanup(func() { requireNFDModule = true })

	enabled := true
	payload := moduleConfigSnapshotPayload{
		Spec: moduleConfigSnapshotSpec{
			Enabled: ptr.To(enabled),
		},
	}
	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(target any) error {
		return json.Unmarshal(mustMarshal(payload), target)
	})

	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(key string) []pkg.Snapshot {
		if key != moduleConfigSnapshot {
			t.Fatalf("unexpected snapshot key: %s", key)
		}
		return []pkg.Snapshot{snapshot}
	})

	nodeFeatureEnsurer = func(pkg.PatchCollector) error {
		return errors.New("rule error")
	}

	values := map[string]any{
		"global": map[string]any{
			"enabledModules": []any{"node-feature-discovery"},
		},
	}

	pc := mock.NewPatchCollectorMock(t)
	pc.CreateIfNotExistsMock.Set(func(any) {})

	input, patchable := newHookInput(t, snapshots, pc, values)

	if err := handleNodeFeatureRuleSync(context.Background(), input); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	patch := findPatchByPath(t, patchable, settings.InternalNodeFeatureRulePath)
	status := decodePatchMap(t, patch.Value)
	if !strings.Contains(status["error"].(string), "rule error") {
		t.Fatalf("unexpected error payload: %#v", status)
	}
}

func TestEnsureNodeFeatureRuleMetadataInitialization(t *testing.T) {
	resetSeams()

	pc := mock.NewPatchCollectorMock(t)
	var captured any
	pc.CreateOrUpdateMock.Set(func(obj any) { captured = obj })

	if err := ensureNodeFeatureRule(pc); err != nil {
		t.Fatalf("ensureNodeFeatureRule returned error: %v", err)
	}

	unstr, ok := captured.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("unexpected type: %T", captured)
	}
	if unstr.GetLabels()["app.kubernetes.io/name"] != settings.ModuleName {
		t.Fatalf("missing module label: %v", unstr.GetLabels())
	}
}

func TestEnsureNodeFeatureRuleUnmarshalError(t *testing.T) {
	resetSeams()

	defer func() { yamlUnmarshal = defaultYAMLUnmarshal }()
	yamlUnmarshal = func([]byte, any) error {
		return errors.New("boom")
	}

	if err := ensureNodeFeatureRule(mock.NewPatchCollectorMock(t)); err == nil || !strings.Contains(err.Error(), "decode NodeFeatureRule manifest") {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

func TestEnsureNamespaceSetsLabels(t *testing.T) {
	pc := mock.NewPatchCollectorMock(t)
	var captured any
	pc.CreateIfNotExistsMock.Set(func(obj any) { captured = obj })

	if err := ensureNamespace(pc); err != nil {
		t.Fatalf("ensureNamespace returned error: %v", err)
	}

	unstr, ok := captured.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("unexpected type: %T", captured)
	}
	if unstr.GetName() != settings.ModuleNamespace {
		t.Fatalf("unexpected namespace: %s", unstr.GetName())
	}
}

func TestCleanupResources(t *testing.T) {
	pc := mock.NewPatchCollectorMock(t)
	var deletes []string
	pc.DeleteMock.Set(func(apiVersion, kind, namespace, name string) {
		deletes = append(deletes, fmt.Sprintf("%s/%s/%s/%s", apiVersion, kind, namespace, name))
	})
	cleanupResources(pc)

	expected := map[string]struct{}{
		"nfd.k8s-sigs.io/v1alpha1/NodeFeatureRule//" + settings.NodeFeatureRuleName: {},
		"v1/Namespace//" + settings.ModuleNamespace:                                 {},
	}
	for _, delete := range deletes {
		if _, ok := expected[delete]; !ok {
			t.Fatalf("unexpected delete: %s", delete)
		}
	}
}

func TestEnsureManagedLabelsInitializesMetadata(t *testing.T) {
	obj := map[string]any{}
	ensureManagedLabels(obj)

	meta, ok := obj["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata not created: %#v", obj)
	}
	labels, ok := meta["labels"].(map[string]any)
	if !ok {
		t.Fatalf("labels not created: %#v", meta)
	}
	if labels["app.kubernetes.io/name"] != settings.ModuleName {
		t.Fatalf("module label missing: %#v", labels)
	}
}

func TestEnsureManagedLabelsPreservesExisting(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{
				"existing": "true",
			},
		},
	}
	ensureManagedLabels(obj)

	labels := obj["metadata"].(map[string]any)["labels"].(map[string]any)
	if labels["existing"] != "true" {
		t.Fatalf("existing label lost: %#v", labels)
	}
	if labels["app.kubernetes.io/managed-by"] != "deckhouse" {
		t.Fatalf("managed-by label missing: %#v", labels)
	}
}

func TestIsModuleEnabled(t *testing.T) {
	cases := []struct {
		name    string
		result  gjson.Result
		enabled bool
	}{
		{
			name:    "string",
			result:  gjson.Parse(`"node-feature-discovery"`),
			enabled: true,
		},
		{
			name:    "array",
			result:  gjson.Parse(`["foo","node-feature-discovery"]`),
			enabled: true,
		},
		{
			name:    "array-miss",
			result:  gjson.Parse(`["foo","bar"]`),
			enabled: false,
		},
		{
			name:    "missing",
			result:  gjson.Parse(`"other"`),
			enabled: false,
		},
		{
			name:    "empty",
			result:  gjson.Result{},
			enabled: false,
		},
	}

	for _, tc := range cases {
		if isModuleEnabled(tc.result, "node-feature-discovery") != tc.enabled {
			t.Fatalf("case %s: expected %v", tc.name, tc.enabled)
		}
	}
}

func findPatchByPath(t *testing.T, values *patchablevalues.PatchableValues, dotPath string) *utils.ValuesPatchOperation {
	t.Helper()

	want := expectedPatchPath(dotPath)
	for _, patch := range values.GetPatches() {
		if patch.Path == want {
			return patch
		}
	}
	t.Fatalf("patch for %s not found, patches: %#v", dotPath, values.GetPatches())
	return nil
}

func decodePatchMap(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode patch value: %v", err)
	}
	return out
}

func expectedPatchPath(path string) string {
	return "/" + strings.ReplaceAll(path, ".", "/")
}

func mustMarshal(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}
