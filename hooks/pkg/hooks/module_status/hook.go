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
	"strings"

	"github.com/tidwall/gjson"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/hooks/pkg/settings"
)

const (
	conditionTypePrereq = "PrerequisiteNotMet"
	reasonNFDDisabled   = "NodeFeatureDiscoveryDisabled"

	validationSource = "module-status/prerequisite"
)

var _ = registry.RegisterFunc(&pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 12},
	Queue:        settings.ModuleQueue,
}, handleModuleStatus)

func handleModuleStatus(_ context.Context, input *pkg.HookInput) error {
	cfg := input.Values.Get(settings.InternalModuleConfigPath)
	if !cfg.Exists() || cfg.Type == gjson.Null || !cfg.Get("enabled").Bool() {
		input.Values.Remove(settings.InternalModuleConditionsPath)
		clearValidationError(input)
		return nil
	}

	var (
		conditions        []map[string]any
		validationMessage []string
	)

	if !isModuleEnabled(input.Values.Get("global.enabledModules"), "node-feature-discovery") {
		msg := "Module gpu-control-plane requires the node-feature-discovery module to be enabled"
		conditions = append(conditions, map[string]any{
			"type":    conditionTypePrereq,
			"status":  "False",
			"reason":  reasonNFDDisabled,
			"message": msg,
		})
		validationMessage = append(validationMessage, msg)
	}

	if len(conditions) == 0 {
		input.Values.Remove(settings.InternalModuleConditionsPath)
		clearValidationError(input)
		return nil
	}

	input.Values.Set(settings.InternalModuleConditionsPath, conditions)

	if len(validationMessage) == 0 {
		clearValidationError(input)
		return nil
	}

	setValidationError(input, strings.Join(validationMessage, "; "))
	return nil
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

func setValidationError(input *pkg.HookInput, message string) {
	current := input.Values.Get(settings.InternalModuleValidationPath)
	if current.Exists() && current.Type != gjson.Null {
		if current.IsObject() && current.Get("source").Str != "" && current.Get("source").Str != validationSource {
			prev := strings.TrimSpace(current.Get("error").Str)
			if prev != "" {
				message = prev + "; " + message
			}
		}
	}
	input.Values.Set(settings.InternalModuleValidationPath, map[string]any{
		"error":  message,
		"source": validationSource,
	})
}

func clearValidationError(input *pkg.HookInput) {
	current := input.Values.Get(settings.InternalModuleValidationPath)
	if !current.Exists() || current.Type == gjson.Null || !current.IsObject() {
		return
	}
	if current.Get("source").Str == validationSource {
		input.Values.Remove(settings.InternalModuleValidationPath)
	}
}
