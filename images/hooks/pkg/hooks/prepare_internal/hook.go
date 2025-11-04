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

package prepare_internal

import (
	"context"

	pkg "github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"k8s.io/utils/ptr"

	"hooks/pkg/settings"
)

const (
	moduleSnapshot      = "module-config"
	moduleConfigJQQuery = `{"enabled": .spec.enabled}`
)

var _ = registry.RegisterFunc(&pkg.HookConfig{
	Queue:        settings.ModuleQueue,
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       moduleSnapshot,
			APIVersion: "deckhouse.io/v1alpha1",
			Kind:       "ModuleConfig",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{settings.ModuleName},
			},
			ExecuteHookOnSynchronization: ptr.To(true),
			ExecuteHookOnEvents:          ptr.To(true),
			JqFilter:                     moduleConfigJQQuery,
		},
	},
}, handle)

func handle(_ context.Context, input *pkg.HookInput) error {
	ensureMap(input.Values, settings.ModuleValuesName)
	ensureMap(input.Values, settings.ModuleValuesName+".internal")
	ensureMap(input.Values, settings.ModuleValuesName+".internal.rootCA")
	ensureMap(input.Values, settings.ModuleValuesName+".internal.controller.cert")

	snapshot := input.Snapshots.Get(moduleSnapshot)
	if len(snapshot) == 0 {
		resetModuleConfig(input.Values, settings.ModuleValuesName)
		return nil
	}

	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if err := snapshot[0].UnmarshalTo(&payload); err != nil {
		return err
	}

	setModuleConfig(input.Values, settings.ModuleValuesName, map[string]any{
		"enabled": payload.Enabled,
	})

	return nil
}

func ensureMap(values pkg.OutputPatchableValuesCollector, path string) {
	if values.Get(path).Exists() {
		return
	}
	values.Set(path, map[string]any{})
}

func setModuleConfig(values pkg.OutputPatchableValuesCollector, moduleName string, payload map[string]any) {
	values.Set(moduleName+".internal.moduleConfig", payload)
}

func resetModuleConfig(values pkg.OutputPatchableValuesCollector, moduleName string) {
	values.Remove(moduleName + ".internal.moduleConfig")
}
