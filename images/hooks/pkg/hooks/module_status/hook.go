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

	"hooks/pkg/settings"
)

const (
	conditionTypePrereq       = "PrerequisiteNotMet"
	reasonNodeFeatureRuleFail = "NodeFeatureRuleApplyFailed"
	reasonNFDDisabled         = "NodeFeatureDiscoveryDisabled"

	validationSource = "module-status/prerequisite"
)

var _ = registry.RegisterFunc(&pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 12},
	Queue:        settings.ModuleQueue,
}, handleModuleStatus)

var requireNFDModule = false

func handleModuleStatus(_ context.Context, input *pkg.HookInput) error {
	cfg := input.Values.Get(settings.InternalModuleConfigPath)
	if !cfg.Exists() || cfg.Type == gjson.Null || !cfg.Get("enabled").Bool() {
		input.Values.Remove(settings.InternalModuleConditionsPath)
		clearValidationError(input)
		return nil
	}

	ensureInternalMap(input, settings.InternalMetricsPath)
	ensureInternalMap(input, settings.InternalMetricsCertPath)

	var conditions []map[string]any

	if requireNFDModule && !isModuleEnabled(input.Values.Get("global.enabledModules"), "node-feature-discovery") {
		msg := settings.NFDDependencyErrorMessage
		conditions = append(conditions, map[string]any{
			"type":    conditionTypePrereq,
			"status":  "False",
			"reason":  reasonNFDDisabled,
			"message": msg,
		})
	} else if msg := strings.TrimSpace(input.Values.Get(settings.InternalNodeFeatureRulePath + ".error").Str); msg != "" {
		conditions = append(conditions, map[string]any{
			"type":    conditionTypePrereq,
			"status":  "False",
			"reason":  reasonNodeFeatureRuleFail,
			"message": msg,
		})
	}

	if len(conditions) == 0 {
		input.Values.Remove(settings.InternalModuleConditionsPath)
		clearValidationError(input)
		return nil
	}

	input.Values.Set(settings.InternalModuleConditionsPath, conditions)

	setValidationError(input, conditions[0]["message"].(string))
	return nil
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

func ensureInternalMap(input *pkg.HookInput, path string) {
	current := input.Values.Get(path)
	if current.Exists() && current.Type != gjson.Null && current.IsObject() {
		return
	}
	input.Values.Set(path, map[string]any{})
}
