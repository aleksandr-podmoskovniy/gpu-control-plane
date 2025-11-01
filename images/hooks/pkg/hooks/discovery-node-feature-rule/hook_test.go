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
	"github.com/deckhouse/module-sdk/testing/mock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

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
}

func TestHandleBootstrapResourcesModuleDisabled(t *testing.T) {
	resetSeams()

	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(key string) []pkg.Snapshot {
		if key != moduleConfigSnapshot {
			t.Fatalf("unexpected snapshot key: %s", key)
		}
		return nil
	})

	patchCollector := mock.NewPatchCollectorMock(t)
	var deletes []string
	patchCollector.DeleteMock.Set(func(apiVersion, kind, namespace, name string) {
		deletes = append(deletes, fmt.Sprintf("%s/%s/%s", apiVersion, kind, name))
	})

	input, patchable := newHookInput(t, snapshots, patchCollector, map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"bootstrap": map[string]any{
					"nodeFeatureRule": map[string]any{"name": "old"},
				},
			},
		},
	})

	if err := handleNodeFeatureRuleSync(context.Background(), input); err != nil {
		t.Fatalf("handleNodeFeatureRuleSync returned error: %v", err)
	}

	expectedDeletes := map[string]struct{}{
		"nfd.k8s-sigs.io/v1alpha1/NodeFeatureRule/" + settings.NodeFeatureRuleName: {},
		"v1/Namespace/" + settings.ModuleNamespace:                                 {},
	}
	for _, delete := range deletes {
		if _, ok := expectedDeletes[delete]; !ok {
			t.Fatalf("unexpected delete operation: %s", delete)
		}
	}

	patches := patchable.GetPatches()
	found := false
	for _, patch := range patches {
		if patch.Op == "remove" && patch.Path == expectedPatchPath(settings.InternalBootstrapPath) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected removal patch for %s, patches: %#v", settings.InternalBootstrapPath, patches)
	}
}

func TestHandleBootstrapResourcesModuleEnabled(t *testing.T) {
	resetSeams()

	enabled := true
	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(target any) error {
		payload, ok := target.(*moduleConfigSnapshotPayload)
		if !ok {
			t.Fatalf("unexpected payload type %T", target)
		}
		payload.Spec.Enabled = &enabled
		return nil
	})

	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(key string) []pkg.Snapshot {
		if key != moduleConfigSnapshot {
			t.Fatalf("unexpected snapshot key: %s", key)
		}
		return []pkg.Snapshot{snapshot}
	})

	patchCollector := mock.NewPatchCollectorMock(t)
	var namespaces []*unstructured.Unstructured
	patchCollector.CreateIfNotExistsMock.Set(func(obj any) {
		unstr, ok := obj.(*unstructured.Unstructured)
		if !ok {
			t.Fatalf("expected *unstructured.Unstructured, got %T", obj)
		}
		namespaces = append(namespaces, unstr)
	})

	var createOrUpdateCalls []*unstructured.Unstructured
	patchCollector.CreateOrUpdateMock.Set(func(obj any) {
		unstr, ok := obj.(*unstructured.Unstructured)
		if !ok {
			t.Fatalf("expected *unstructured.Unstructured, got %T", obj)
		}
		createOrUpdateCalls = append(createOrUpdateCalls, unstr)
	})

	input, patchable := newHookInput(t, snapshots, patchCollector, map[string]any{})

	if err := handleNodeFeatureRuleSync(context.Background(), input); err != nil {
		t.Fatalf("handleNodeFeatureRuleSync returned error: %v", err)
	}

	if len(namespaces) != 1 || namespaces[0].GetName() != settings.ModuleNamespace {
		t.Fatalf("namespace creation mismatch: %+v", namespaces)
	}

	if len(createOrUpdateCalls) != 1 {
		t.Fatalf("expected node feature rule creation, got %d calls", len(createOrUpdateCalls))
	}
	rule := createOrUpdateCalls[0]
	if rule.GetKind() != "NodeFeatureRule" {
		t.Fatalf("unexpected object kind: %s", rule.GetKind())
	}
	labels := rule.GetLabels()
	if labels["app.kubernetes.io/name"] != settings.ModuleName {
		t.Fatalf("unexpected label: %v", labels)
	}

	var bootstrap map[string]any
	for _, patch := range patchable.GetPatches() {
		if patch.Op == "add" && patch.Path == expectedPatchPath(settings.InternalBootstrapPath) {
			if err := json.Unmarshal(patch.Value, &bootstrap); err != nil {
				t.Fatalf("unmarshal bootstrap payload: %v", err)
			}
		}
	}
	if bootstrap == nil {
		t.Fatal("expected bootstrap values to be set")
	}
	nodeFeature, ok := bootstrap["nodeFeatureRule"].(map[string]any)
	if !ok || nodeFeature["name"] != settings.NodeFeatureRuleName {
		t.Fatalf("unexpected nodeFeatureRule payload: %#v", bootstrap)
	}
}

func TestHandleBootstrapResourcesModuleEnabledUnset(t *testing.T) {
	resetSeams()

	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(target any) error {
		payload := target.(*moduleConfigSnapshotPayload)
		payload.Spec.Enabled = nil
		return nil
	})

	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(key string) []pkg.Snapshot {
		if key != moduleConfigSnapshot {
			t.Fatalf("unexpected snapshot key: %s", key)
		}
		return []pkg.Snapshot{snapshot}
	})

	patchCollector := mock.NewPatchCollectorMock(t)
	var deletes []string
	patchCollector.DeleteMock.Set(func(apiVersion, kind, namespace, name string) {
		deletes = append(deletes, fmt.Sprintf("%s/%s/%s", apiVersion, kind, name))
	})

	values := map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"bootstrap": map[string]any{"nodeFeatureRule": map[string]any{"name": "old"}},
			},
		},
	}

	input, patchable := newHookInput(t, snapshots, patchCollector, values)

	if err := handleNodeFeatureRuleSync(context.Background(), input); err != nil {
		t.Fatalf("handleNodeFeatureRuleSync returned error: %v", err)
	}

	if len(deletes) != 2 {
		t.Fatalf("unexpected delete calls: %v", deletes)
	}

	removed := false
	for _, patch := range patchable.GetPatches() {
		if patch.Op == "remove" && patch.Path == expectedPatchPath(settings.InternalBootstrapPath) {
			removed = true
		}
	}
	if !removed {
		t.Fatalf("expected bootstrap removal, patches: %#v", patchable.GetPatches())
	}
}

func TestHandleBootstrapResourcesSnapshotError(t *testing.T) {
	resetSeams()

	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(any) error { return errors.New("decode error") })

	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(string) []pkg.Snapshot { return []pkg.Snapshot{snapshot} })

	patchCollector := mock.NewPatchCollectorMock(t)
	input, _ := newHookInput(t, snapshots, patchCollector, map[string]any{})

	if err := handleNodeFeatureRuleSync(context.Background(), input); err == nil {
		t.Fatal("expected error from handleNodeFeatureRuleSync")
	}
}
func TestHandleBootstrapNamespaceError(t *testing.T) {
	resetSeams()
	namespaceEnsurer = func(pkg.PatchCollector) error { return errors.New("ns error") }
	nodeFeatureEnsurer = func(pkg.PatchCollector) error { return nil }
	enabled := true
	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(target any) error {
		payload := target.(*moduleConfigSnapshotPayload)
		payload.Spec.Enabled = &enabled
		return nil
	})
	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(string) []pkg.Snapshot { return []pkg.Snapshot{snapshot} })
	input, _ := newHookInput(t, snapshots, mock.NewPatchCollectorMock(t), map[string]any{})
	if err := handleNodeFeatureRuleSync(context.Background(), input); err == nil || !strings.Contains(err.Error(), "ns error") {
		t.Fatalf("expected namespace error, got %v", err)
	}
}

func TestEnsureNodeFeatureRuleUnmarshalFailure(t *testing.T) {
	resetSeams()
	yamlUnmarshal = func([]byte, any) error { return errors.New("yaml") }
	defer func() { yamlUnmarshal = defaultYamlUnmarshal }()
	if err := ensureNodeFeatureRule(mock.NewPatchCollectorMock(t)); err == nil || !strings.Contains(err.Error(), "yaml") {
		t.Fatalf("expected yaml error, got %v", err)
	}
}

func TestEnsureNodeFeatureRuleMetadataInitialization(t *testing.T) {
	resetSeams()

	yamlData := fmt.Sprintf(nodeFeatureRuleTemplate, settings.NodeFeatureRuleName)
	var obj map[string]any
	if err := yaml.Unmarshal([]byte(yamlData), &obj); err != nil {
		t.Fatalf("unexpected yaml error: %v", err)
	}

	patchCollector := mock.NewPatchCollectorMock(t)
	var captured any
	patchCollector.CreateOrUpdateMock.Set(func(obj any) { captured = obj })

	if err := ensureNodeFeatureRule(patchCollector); err != nil {
		t.Fatalf("ensureNodeFeatureRule returned error: %v", err)
	}

	unstr, ok := captured.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("expected *unstructured.Unstructured, got %T", captured)
	}
	labels := unstr.GetLabels()
	if labels["app.kubernetes.io/name"] != settings.ModuleName {
		t.Fatalf("expected module label, got %v", labels)
	}
	if labels["app.kubernetes.io/managed-by"] != "deckhouse" {
		t.Fatalf("expected managed-by label, got %v", labels)
	}
}

func TestEnsureNodeFeatureRuleUnmarshalError(t *testing.T) {
	resetSeams()

	nodeFeatureEnsurer = func(pkg.PatchCollector) error {
		return errors.New("inner error")
	}
	namespaceEnsurer = func(pkg.PatchCollector) error {
		return nil
	}

	enabled := true
	snapshot := mock.NewSnapshotMock(t)
	snapshot.UnmarshalToMock.Set(func(target any) error {
		payload := target.(*moduleConfigSnapshotPayload)
		payload.Spec.Enabled = &enabled
		return nil
	})

	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(key string) []pkg.Snapshot {
		if key != moduleConfigSnapshot {
			t.Fatalf("unexpected snapshot key: %s", key)
		}
		return []pkg.Snapshot{snapshot}
	})

	patchCollector := mock.NewPatchCollectorMock(t)
	input, _ := newHookInput(t, snapshots, patchCollector, map[string]any{})

	if err := handleNodeFeatureRuleSync(context.Background(), input); err == nil {
		t.Fatal("expected error from handleNodeFeatureRuleSync when nodeFeatureEnsurer fails")
	}
}

func TestEnsureManagedLabelsExisting(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{"existing": "true"},
		},
	}
	ensureManagedLabels(obj)

	labels := obj["metadata"].(map[string]any)["labels"].(map[string]any)
	if labels["existing"] != "true" {
		t.Fatalf("existing label lost: %v", labels)
	}
	if labels["app.kubernetes.io/name"] != settings.ModuleName {
		t.Fatalf("module label missing: %v", labels)
	}
	if labels["app.kubernetes.io/managed-by"] != "deckhouse" {
		t.Fatalf("managed-by label missing: %v", labels)
	}
}

func TestEnsureNamespace(t *testing.T) {
	pc := mock.NewPatchCollectorMock(t)
	var created any
	pc.CreateIfNotExistsMock.Set(func(obj any) { created = obj })

	if err := ensureNamespace(pc); err != nil {
		t.Fatalf("ensureNamespace returned error: %v", err)
	}

	unstr, ok := created.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("expected *unstructured.Unstructured, got %T", created)
	}
	if unstr.GetName() != settings.ModuleNamespace {
		t.Fatalf("unexpected namespace name: %s", unstr.GetName())
	}
}

func TestCleanupResources(t *testing.T) {
	pc := mock.NewPatchCollectorMock(t)
	var deletes []string
	pc.DeleteMock.Set(func(apiVersion, kind, namespace, name string) {
		deletes = append(deletes, fmt.Sprintf("%s/%s/%s", apiVersion, kind, name))
	})
	cleanupResources(pc)

	expected := map[string]struct{}{
		"nfd.k8s-sigs.io/v1alpha1/NodeFeatureRule/" + settings.NodeFeatureRuleName: {},
		"v1/Namespace/" + settings.ModuleNamespace:                                 {},
	}
	for _, delete := range deletes {
		if _, ok := expected[delete]; !ok {
			t.Fatalf("unexpected delete: %s", delete)
		}
	}
}

func expectedPatchPath(path string) string {
	return "/" + strings.ReplaceAll(path, ".", "/")
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
	if labels["app.kubernetes.io/managed-by"] != "deckhouse" {
		t.Fatalf("managed-by label missing: %#v", labels)
	}
}
