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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"text/template"

	pkg "github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"
	"github.com/deckhouse/module-sdk/pkg/utils"
	"github.com/deckhouse/module-sdk/testing/mock"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"

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
	nodeFeatureEnsurer = ensureNodeFeatureRule
	yamlUnmarshal = defaultYAMLUnmarshal
}

func TestHandleNodeFeatureRuleSyncModuleDisabled(t *testing.T) {
	resetSeams()
	prev := requireNFDModule
	requireNFDModule = false
	t.Cleanup(func() { requireNFDModule = prev })

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
	prev := requireNFDModule
	requireNFDModule = true
	t.Cleanup(func() { requireNFDModule = prev })

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
	prev := requireNFDModule
	requireNFDModule = false
	t.Cleanup(func() { requireNFDModule = prev })

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

	if len(created) != 1 {
		t.Fatalf("expected NodeFeatureRule creation, got %d", len(created))
	}
	unstr, ok := created[0].(*unstructured.Unstructured)
	if !ok || unstr.GetKind() != "NodeFeatureRule" {
		t.Fatalf("unexpected object created: %#v", created[0])
	}
	if unstr.GetName() != settings.NodeFeatureRuleName {
		t.Fatalf("unexpected NodeFeatureRule name: %s", unstr.GetName())
	}
	labels := unstr.GetLabels()
	if labels["app.kubernetes.io/name"] != settings.ModuleName {
		t.Fatalf("module label missing: %v", labels)
	}

	nfr := &nfdv1alpha1.NodeFeatureRule{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstr.Object, nfr); err != nil {
		t.Fatalf("convert unstructured NodeFeatureRule: %v", err)
	}
	if len(nfr.Spec.Rules) == 0 {
		t.Fatalf("expected at least one rule")
	}
	gpuRule := nfr.Spec.Rules[0]
	if len(gpuRule.MatchFeatures) != 1 {
		t.Fatalf("expected single matchFeatures entry for GPU rule, got %d", len(gpuRule.MatchFeatures))
	}
	expressions := gpuRule.MatchFeatures[0].MatchExpressions
	if expressions == nil {
		t.Fatalf("gpu rule expressions nil")
	}
	expectMatch := func(name string, op nfdv1alpha1.MatchOp, values []string) {
		expr, ok := (*expressions)[name]
		if !ok {
			t.Fatalf("expression %s missing", name)
		}
		if expr.Op != op {
			t.Fatalf("expression %s expected op %s, got %s", name, op, expr.Op)
		}
		if values == nil {
			if len(expr.Value) != 0 {
				t.Fatalf("expression %s expected empty value, got %#v", name, expr.Value)
			}
		} else if !reflect.DeepEqual([]string(expr.Value), values) {
			t.Fatalf("expression %s values mismatch: want %v got %v", name, values, expr.Value)
		}
	}
	expectMatch("vendor", nfdv1alpha1.MatchIn, []string{"10de"})
	expectMatch("class", nfdv1alpha1.MatchIn, []string{"0300", "0302"})

	findRule := func(name string) *nfdv1alpha1.Rule {
		for i := range nfr.Spec.Rules {
			if nfr.Spec.Rules[i].Name == name {
				return &nfr.Spec.Rules[i]
			}
		}
		t.Fatalf("rule %s not found", name)
		return nil
	}

	checkExists := func(ruleName, feature, expression string) {
		rule := findRule(ruleName)
		if len(rule.MatchFeatures) != 1 {
			t.Fatalf("rule %s expected single match feature, got %d", ruleName, len(rule.MatchFeatures))
		}
		if rule.MatchFeatures[0].Feature != feature {
			t.Fatalf("rule %s expected feature %s, got %s", ruleName, feature, rule.MatchFeatures[0].Feature)
		}
		exprs := rule.MatchFeatures[0].MatchExpressions
		if exprs == nil {
			t.Fatalf("rule %s expressions nil", ruleName)
		}
		expr, ok := (*exprs)[expression]
		if !ok {
			t.Fatalf("rule %s missing expression %s", ruleName, expression)
		}
		if expr.Op != nfdv1alpha1.MatchExists {
			t.Fatalf("rule %s expression %s expected Exists, got %s", ruleName, expression, expr.Op)
		}
		if len(expr.Value) != 0 {
			t.Fatalf("rule %s expression %s expected empty value, got %#v", ruleName, expression, expr.Value)
		}
	}

	checkExists("deckhouse.gpu.nvidia-driver", "kernel.loadedmodule", "nvidia")
	checkExists("deckhouse.gpu.nvidia-modeset", "kernel.loadedmodule", "nvidia_modeset")
	checkExists("deckhouse.gpu.nvidia-uvm", "kernel.loadedmodule", "nvidia_uvm")
	checkExists("deckhouse.gpu.nvidia-drm", "kernel.loadedmodule", "nvidia_drm")

	kernelRule := findRule("deckhouse.system.kernel-os")
	if len(kernelRule.MatchFeatures) != 2 {
		t.Fatalf("kernel rule expected two match features, got %d", len(kernelRule.MatchFeatures))
	}
	var kernelExprs, osExprs *nfdv1alpha1.MatchExpressionSet
	for _, mf := range kernelRule.MatchFeatures {
		switch mf.Feature {
		case "kernel.version":
			kernelExprs = mf.MatchExpressions
		case "system.osrelease":
			osExprs = mf.MatchExpressions
		default:
			t.Fatalf("unexpected feature %s in kernel rule", mf.Feature)
		}
	}
	if kernelExprs == nil || osExprs == nil {
		t.Fatalf("kernel/os expressions missing")
	}
	for _, key := range []string{"major", "minor", "full"} {
		expr, ok := (*kernelExprs)[key]
		if !ok || expr.Op != nfdv1alpha1.MatchExists {
			t.Fatalf("kernel expression %s invalid: %+v", key, expr)
		}
		if len(expr.Value) != 0 {
			t.Fatalf("kernel expression %s expected empty value, got %#v", key, expr.Value)
		}
	}
	for _, key := range []string{"ID", "VERSION_ID"} {
		expr, ok := (*osExprs)[key]
		if !ok || expr.Op != nfdv1alpha1.MatchExists {
			t.Fatalf("os expression %s invalid: %+v", key, expr)
		}
		if len(expr.Value) != 0 {
			t.Fatalf("os expression %s expected empty value, got %#v", key, expr.Value)
		}
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
	prev := requireNFDModule
	requireNFDModule = false
	t.Cleanup(func() { requireNFDModule = prev })

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
	prev := requireNFDModule
	requireNFDModule = false
	t.Cleanup(func() { requireNFDModule = prev })

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
	for _, patch := range patchable.GetPatches() {
		if patch.Path == expectedPatchPath(settings.InternalNodeFeatureRulePath) {
			t.Fatalf("did not expect nodeFeatureRule patch, got %+v", patch)
		}
	}
}

func TestHandleNodeFeatureRuleSyncRuleError(t *testing.T) {
	resetSeams()
	prev := requireNFDModule
	requireNFDModule = true
	t.Cleanup(func() { requireNFDModule = prev })

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
	pc.CreateIfNotExistsMock.Optional()

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

func TestEnsureNamespaceCreatesNamespaceWithLabels(t *testing.T) {
	pc := mock.NewPatchCollectorMock(t)
	var captured any
	pc.CreateIfNotExistsMock.Set(func(obj any) { captured = obj })

	if err := ensureNamespace(pc); err != nil {
		t.Fatalf("ensureNamespace returned error: %v", err)
	}

	unstr, ok := captured.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("unexpected object type: %T", captured)
	}
	if unstr.GetName() != settings.ModuleNamespace {
		t.Fatalf("unexpected namespace name: %s", unstr.GetName())
	}
	labels := unstr.GetLabels()
	if labels["app.kubernetes.io/name"] != settings.ModuleName {
		t.Fatalf("unexpected module label: %#v", labels)
	}
	if labels["app.kubernetes.io/managed-by"] != "deckhouse" {
		t.Fatalf("unexpected managed-by label: %#v", labels)
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

func TestCleanupResources(t *testing.T) {
	pc := mock.NewPatchCollectorMock(t)
	var deletes []string
	pc.DeleteMock.Set(func(apiVersion, kind, namespace, name string) {
		deletes = append(deletes, fmt.Sprintf("%s/%s/%s/%s", apiVersion, kind, namespace, name))
	})
	cleanupResources(pc)

	expected := map[string]struct{}{
		"nfd.k8s-sigs.io/v1alpha1/NodeFeatureRule//" + settings.NodeFeatureRuleName: {},
	}
	for _, delete := range deletes {
		if _, ok := expected[delete]; !ok {
			t.Fatalf("unexpected delete: %s", delete)
		}
	}
}

func TestKernelLabelsTemplateHandlesMapPayload(t *testing.T) {
	rule := buildNodeFeatureRuleFromTemplate(t)
	kernelRule := findRuleByName(t, rule, "deckhouse.system.kernel-os")

	data := map[string]any{
		"kernel.version": map[string]any{
			"full":  "5.15.0-1075-azure",
			"major": 5,
			"minor": 15,
		},
		"system.osrelease": map[string]any{
			"ID":         "ubuntu",
			"VERSION_ID": "22.04",
		},
	}

	rendered := executeTemplate(t, kernelRule.LabelsTemplate, data)
	assertContains(t, rendered, "gpu.deckhouse.io/kernel.version.full=5.15.0-1075-azure")
	assertContains(t, rendered, "gpu.deckhouse.io/kernel.version.major=5")
	assertContains(t, rendered, "gpu.deckhouse.io/kernel.version.minor=15")
	assertContains(t, rendered, "gpu.deckhouse.io/os.id=ubuntu")
	assertContains(t, rendered, "gpu.deckhouse.io/os.version_id=22.04")
}

func TestKernelLabelsTemplateHandlesLegacySlicePayload(t *testing.T) {
	rule := buildNodeFeatureRuleFromTemplate(t)
	kernelRule := findRuleByName(t, rule, "deckhouse.system.kernel-os")

	data := map[string]any{
		"kernel.version": []map[string]any{
			{
				"Attributes": map[string]any{
					"full":  "6.6.1-custom",
					"major": "6",
					"minor": "6",
				},
			},
		},
		"system.osrelease": []map[string]any{
			{
				"Attributes": map[string]any{
					"ID":         "talos",
					"VERSION_ID": "1.6.7",
				},
			},
		},
	}

	rendered := executeTemplate(t, kernelRule.LabelsTemplate, data)
	assertContains(t, rendered, "gpu.deckhouse.io/kernel.version.full=6.6.1-custom")
	assertContains(t, rendered, "gpu.deckhouse.io/kernel.version.major=6")
	assertContains(t, rendered, "gpu.deckhouse.io/kernel.version.minor=6")
	assertContains(t, rendered, "gpu.deckhouse.io/os.id=talos")
	assertContains(t, rendered, "gpu.deckhouse.io/os.version_id=1.6.7")
}

func executeTemplate(t *testing.T, tpl string, data map[string]any) string {
	t.Helper()
	parsed, err := template.New("labels").Parse(tpl)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		t.Fatalf("render template: %v", err)
	}
	return buf.String()
}

func buildNodeFeatureRuleFromTemplate(t *testing.T) *nfdv1alpha1.NodeFeatureRule {
	t.Helper()
	pc := mock.NewPatchCollectorMock(t)
	var created any
	pc.CreateOrUpdateMock.Set(func(obj any) { created = obj })

	if err := ensureNodeFeatureRule(pc); err != nil {
		t.Fatalf("ensureNodeFeatureRule: %v", err)
	}

	unstr, ok := created.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("unexpected type %T", created)
	}

	rule := &nfdv1alpha1.NodeFeatureRule{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstr.Object, rule); err != nil {
		t.Fatalf("convert NodeFeatureRule: %v", err)
	}
	return rule
}

func findRuleByName(t *testing.T, rule *nfdv1alpha1.NodeFeatureRule, name string) *nfdv1alpha1.Rule {
	t.Helper()
	for i := range rule.Spec.Rules {
		if rule.Spec.Rules[i].Name == name {
			return &rule.Spec.Rules[i]
		}
	}
	t.Fatalf("rule %s not found", name)
	return nil
}

func assertContains(t *testing.T, rendered, fragment string) {
	t.Helper()
	if !strings.Contains(rendered, fragment) {
		t.Fatalf("expected template output to contain %q, got %q", fragment, rendered)
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
