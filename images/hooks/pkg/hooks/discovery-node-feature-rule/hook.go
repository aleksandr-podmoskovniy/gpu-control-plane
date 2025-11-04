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
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"

	"hooks/pkg/settings"
)

const (
	moduleConfigSnapshot = "module-config"
	moduleConfigJQFilter = `{"spec":{"enabled":.spec.enabled}}`
)

type moduleConfigSnapshotSpec struct {
	Enabled *bool `json:"enabled"`
}

type moduleConfigSnapshotPayload struct {
	Spec moduleConfigSnapshotSpec `json:"spec"`
}

const nodeFeatureRuleTemplate = `
apiVersion: nfd.k8s-sigs.io/v1alpha1
kind: NodeFeatureRule
metadata:
  name: %s
spec:
  rules:
    - name: deckhouse.gpu.nvidia
      matchFeatures:
        - feature: pci.device
          matchExpressions:
            vendor:
              op: In
              value: ["10de"]
            class:
              op: In
              value: ["0300", "0302"]
      labelsTemplate: |
        {{- $devices := index .Match.Features "pci.device" -}}
        {{- if $devices }}
        gpu.deckhouse.io/present=true
        gpu.deckhouse.io/device-count={{ len $devices }}
        {{- range $idx, $dev := $devices }}
        {{- $slot := printf "%%02d" $idx -}}
        {{- $attrs := $dev.Attributes -}}
        gpu.deckhouse.io/device.{{ $slot }}.vendor={{ index $attrs "vendor" }}
        gpu.deckhouse.io/device.{{ $slot }}.device={{ index $attrs "device" }}
        gpu.deckhouse.io/device.{{ $slot }}.class={{ index $attrs "class" }}
        {{- end }}
        {{- end }}
    - name: deckhouse.gpu.nvidia-driver
      matchFeatures:
        - feature: kernel.loadedmodule
          matchExpressions:
            nvidia:
              op: Exists
      labels:
        gpu.deckhouse.io/driver.name: nvidia
        gpu.deckhouse.io/driver.module.nvidia: "true"
    - name: deckhouse.gpu.nvidia-modeset
      matchFeatures:
        - feature: kernel.loadedmodule
          matchExpressions:
            nvidia_modeset:
              op: Exists
      labels:
        gpu.deckhouse.io/driver.module.nvidia_modeset: "true"
    - name: deckhouse.gpu.nvidia-uvm
      matchFeatures:
        - feature: kernel.loadedmodule
          matchExpressions:
            nvidia_uvm:
              op: Exists
      labels:
        gpu.deckhouse.io/driver.module.nvidia_uvm: "true"
    - name: deckhouse.gpu.nvidia-drm
      matchFeatures:
        - feature: kernel.loadedmodule
          matchExpressions:
            nvidia_drm:
              op: Exists
      labels:
        gpu.deckhouse.io/driver.module.nvidia_drm: "true"
    - name: deckhouse.system.kernel-os
      matchFeatures:
        - feature: kernel.version
          matchExpressions:
            major:
              op: Exists
            minor:
              op: Exists
            full:
              op: Exists
        - feature: system.osrelease
          matchExpressions:
            ID:
              op: Exists
            VERSION_ID:
              op: Exists
      labelsTemplate: |
        {{- $kernel := index .Match.Features "kernel.version" -}}
        {{- $os := index .Match.Features "system.osrelease" -}}
        {{- $kattrs := (index $kernel 0).Attributes -}}
        {{- $oattrs := (index $os 0).Attributes -}}
        gpu.deckhouse.io/kernel.version.full={{ index $kattrs "full" }}
        gpu.deckhouse.io/kernel.version.major={{ index $kattrs "major" }}
        gpu.deckhouse.io/kernel.version.minor={{ index $kattrs "minor" }}
        gpu.deckhouse.io/os.id={{ index $oattrs "ID" }}
        gpu.deckhouse.io/os.version_id={{ index $oattrs "VERSION_ID" }}
`

var (
	namespaceEnsurer   = ensureNamespace
	nodeFeatureEnsurer = ensureNodeFeatureRule
	yamlUnmarshal      = defaultYAMLUnmarshal
	requireNFDModule   = false
)

func defaultYAMLUnmarshal(data []byte, out any) error {
	return yaml.Unmarshal(data, out)
}

var _ = registry.RegisterFunc(&pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 18},
	Queue:        settings.ModuleQueue,
	Kubernetes: []pkg.KubernetesConfig{
		{
			APIVersion: "deckhouse.io/v1alpha1",
			Kind:       "ModuleConfig",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{settings.ModuleName},
			},
			ExecuteHookOnSynchronization: ptr.To(true),
			ExecuteHookOnEvents:          ptr.To(true),
			JqFilter:                     moduleConfigJQFilter,
		},
		{
			APIVersion: "nfd.k8s-sigs.io/v1alpha1",
			Kind:       "NodeFeatureRule",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{settings.NodeFeatureRuleName},
			},
			ExecuteHookOnSynchronization: ptr.To(true),
			ExecuteHookOnEvents:          ptr.To(true),
			AllowFailure:                 ptr.To(true),
			WaitForSynchronization:       ptr.To(false),
		},
	},
}, handleNodeFeatureRuleSync)

func handleNodeFeatureRuleSync(_ context.Context, input *pkg.HookInput) error {
	if requireNFDModule && !isModuleEnabled(input.Values.Get("global.enabledModules"), "node-feature-discovery") {
		cleanupResources(input.PatchCollector)
		input.Values.Remove(settings.InternalNodeFeatureRulePath)
		return fmt.Errorf(settings.NFDDependencyErrorMessage)
	}

	enabled, err := moduleConfigEnabled(input)
	if err != nil {
		return err
	}

	input.Values.Remove(settings.InternalNodeFeatureRulePath)

	if !enabled {
		cleanupResources(input.PatchCollector)
		return nil
	}

	if err := namespaceEnsurer(input.PatchCollector); err != nil {
		reportNodeFeatureRuleError(input, err)
		return nil
	}
	if err := nodeFeatureEnsurer(input.PatchCollector); err != nil {
		reportNodeFeatureRuleError(input, err)
		return nil
	}

	input.Values.Set(settings.InternalNodeFeatureRulePath, map[string]any{
		"name": settings.NodeFeatureRuleName,
	})

	return nil
}

func moduleConfigEnabled(input *pkg.HookInput) (bool, error) {
	snapshot := input.Snapshots.Get(moduleConfigSnapshot)
	if len(snapshot) == 0 {
		return false, nil
	}

	var payload moduleConfigSnapshotPayload
	if err := snapshot[0].UnmarshalTo(&payload); err != nil {
		return false, fmt.Errorf("decode ModuleConfig/%s: %w", settings.ModuleName, err)
	}

	if payload.Spec.Enabled == nil {
		return false, nil
	}

	return ptr.Deref(payload.Spec.Enabled, false), nil
}

func ensureNamespace(pc pkg.PatchCollector) error {
	namespace := map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]any{
			"name": settings.ModuleNamespace,
			"labels": map[string]any{
				"app.kubernetes.io/name":       settings.ModuleName,
				"app.kubernetes.io/managed-by": "deckhouse",
			},
		},
	}

	pc.CreateIfNotExists(&unstructured.Unstructured{Object: namespace})
	return nil
}

func ensureNodeFeatureRule(pc pkg.PatchCollector) error {
	manifest := fmt.Sprintf(nodeFeatureRuleTemplate, settings.NodeFeatureRuleName)
	var obj map[string]any
	if err := yamlUnmarshal([]byte(manifest), &obj); err != nil {
		return fmt.Errorf("decode NodeFeatureRule manifest: %w", err)
	}

	ensureManagedLabels(obj)

	pc.CreateOrUpdate(&unstructured.Unstructured{Object: obj})
	return nil
}

func cleanupResources(pc pkg.PatchCollector) {
	pc.Delete("nfd.k8s-sigs.io/v1alpha1", "NodeFeatureRule", "", settings.NodeFeatureRuleName)
	pc.Delete("v1", "Namespace", "", settings.ModuleNamespace)
}

func ensureManagedLabels(obj map[string]any) {
	meta, ok := obj["metadata"].(map[string]any)
	if !ok {
		meta = make(map[string]any)
		obj["metadata"] = meta
	}

	labels, ok := meta["labels"].(map[string]any)
	if !ok {
		labels = make(map[string]any)
		meta["labels"] = labels
	}

	labels["app.kubernetes.io/name"] = settings.ModuleName
	labels["app.kubernetes.io/managed-by"] = "deckhouse"
}

func reportNodeFeatureRuleError(input *pkg.HookInput, err error) {
	input.Values.Set(settings.InternalNodeFeatureRulePath, map[string]any{
		"error": err.Error(),
	})
}

func isModuleEnabled(modules gjson.Result, name string) bool {
	if !modules.Exists() {
		return false
	}
	if modules.Type == gjson.String {
		return strings.EqualFold(strings.TrimSpace(modules.Str), name)
	}
	if modules.IsArray() {
		for _, item := range modules.Array() {
			if strings.EqualFold(strings.TrimSpace(item.Str), name) {
				return true
			}
		}
	}
	return false
}
