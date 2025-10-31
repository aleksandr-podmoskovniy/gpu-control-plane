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

package bootstrap_resources

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sync"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/hooks/pkg/settings"
)

const moduleConfigSnapshot = "module-config"

type moduleConfigSpec struct {
	Enabled *bool `json:"enabled"`
}

type moduleConfigSnapshotPayload struct {
	Spec moduleConfigSpec `json:"spec"`
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

//go:embed assets/crds/*.yaml
var crdFS embed.FS

var (
	crdOnce      sync.Once
	crdObjects   []*unstructured.Unstructured
	crdLoadError error
)

var _ = registry.RegisterFunc(&pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 20},
	Queue:        settings.ModuleQueue,
	Kubernetes: []pkg.KubernetesConfig{
		{
			APIVersion: "deckhouse.io/v1alpha1",
			Kind:       "ModuleConfig",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{settings.ModuleName},
			},
		},
	},
}, func(_ context.Context, input *pkg.HookInput) error {
	state, err := moduleConfigEnabled(input)
	if err != nil {
		return err
	}

	if !state {
		cleanupResources(input.PatchCollector)
		input.Values.Remove(settings.InternalBootstrapPath)
		return nil
	}

	if err := ensureCRDs(input.PatchCollector); err != nil {
		return err
	}
	if err := ensureNamespace(input.PatchCollector); err != nil {
		return err
	}
	if err := ensureNodeFeatureRule(input.PatchCollector); err != nil {
		return err
	}

	input.Values.Set(settings.InternalBootstrapPath, map[string]any{
		"nodeFeatureRule": map[string]any{
			"name": settings.NodeFeatureRuleName,
		},
	})

	return nil
})

func moduleConfigEnabled(input *pkg.HookInput) (bool, error) {
	snapshot := input.Snapshots.Get(moduleConfigSnapshot)
	if len(snapshot) == 0 {
		return false, nil
	}

	var mc moduleConfigSnapshotPayload
	if err := snapshot[0].UnmarshalTo(&mc); err != nil {
		return false, fmt.Errorf("decode ModuleConfig/%s: %w", settings.ModuleName, err)
	}

	if mc.Spec.Enabled == nil {
		return false, nil
	}
	return *mc.Spec.Enabled, nil
}

func ensureCRDs(pc pkg.PatchCollector) error {
	objects, err := loadCRDObjects()
	if err != nil {
		return err
	}
	for _, obj := range objects {
		pc.CreateOrUpdate(obj.DeepCopy())
	}
	return nil
}

func loadCRDObjects() ([]*unstructured.Unstructured, error) {
	crdOnce.Do(func() {
		matches, err := fs.Glob(crdFS, "assets/crds/*.yaml")
		if err != nil {
			crdLoadError = fmt.Errorf("list CRDs: %w", err)
			return
		}
		for _, file := range matches {
			data, err := crdFS.ReadFile(file)
			if err != nil {
				crdLoadError = fmt.Errorf("read CRD %s: %w", file, err)
				return
			}
			var payload map[string]any
			if err := yaml.Unmarshal(data, &payload); err != nil {
				crdLoadError = fmt.Errorf("decode CRD %s: %w", file, err)
				return
			}
			crdObjects = append(crdObjects, &unstructured.Unstructured{Object: payload})
		}
	})
	return crdObjects, crdLoadError
}

func ensureNamespace(pc pkg.PatchCollector) error {
	ns := map[string]any{
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
	pc.CreateIfNotExists(&unstructured.Unstructured{Object: ns})
	return nil
}

func ensureNodeFeatureRule(pc pkg.PatchCollector) error {
	manifest := fmt.Sprintf(nodeFeatureRuleTemplate, settings.NodeFeatureRuleName)
	var obj map[string]any
	if err := yaml.Unmarshal([]byte(manifest), &obj); err != nil {
		return fmt.Errorf("decode NodeFeatureRule manifest: %w", err)
	}

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

	pc.CreateOrUpdate(&unstructured.Unstructured{Object: obj})
	return nil
}

func cleanupResources(pc pkg.PatchCollector) {
	pc.Delete("nfd.k8s-sigs.io/v1alpha1", "NodeFeatureRule", "", settings.NodeFeatureRuleName)
	pc.Delete("v1", "Namespace", "", settings.ModuleNamespace)
}
