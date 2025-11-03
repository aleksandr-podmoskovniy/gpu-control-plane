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

package validate_module_config

import (
	"context"
	"fmt"
	"strings"

	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"

	"hooks/pkg/settings"
)

const (
	moduleConfigSnapshot = "module-config"
	moduleConfigJQFilter = `{"spec":{"enabled":.spec.enabled,"settings":.spec.settings}}`
)

type moduleConfigSnapshotSpec struct {
	Enabled  *bool          `json:"enabled"`
	Settings map[string]any `json:"settings"`
}

type moduleConfigSnapshotPayload struct {
	Spec moduleConfigSnapshotSpec `json:"spec"`
}

var moduleConfigFromSnapshotFn = moduleConfigFromSnapshot

var _ = registry.RegisterFunc(&pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 10},
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
	},
}, handleValidateModuleConfig)

func handleValidateModuleConfig(_ context.Context, input *pkg.HookInput) error {
	state, err := moduleConfigFromSnapshotFn(input)
	if err != nil {
		registerValidationError(input, err)
		clearModuleValues(input)
		return nil
	}

	if state == nil {
		clearModuleValues(input)
		input.Values.Remove(settings.InternalModuleValidationPath)
		return nil
	}

	clone := state.Clone()
	sanitized := clone.Sanitized

	input.Values.Remove(settings.InternalModuleValidationPath)

	payload := map[string]any{"enabled": state.Enabled}
	if len(sanitized) > 0 {
		payload["settings"] = sanitized
	}
	input.Values.Set(settings.InternalModuleConfigPath, payload)

	setValueOrRemove(input, settings.ConfigRoot+".managedNodes", sanitized["managedNodes"])
	setValueOrRemove(input, settings.ConfigRoot+".deviceApproval", sanitized["deviceApproval"])
	setValueOrRemove(input, settings.ConfigRoot+".scheduling", sanitized["scheduling"])
	setValueOrRemove(input, settings.ConfigRoot+".inventory", sanitized["inventory"])

	var userHTTPS map[string]any
	if raw, ok := sanitized["https"]; ok {
		if cast, ok := raw.(map[string]any); ok {
			userHTTPS = cast
		}
	}
	if https := resolveHTTPSConfig(input, userHTTPS); https != nil {
		input.Values.Set(settings.HTTPSConfigPath, https)
	} else {
		input.Values.Remove(settings.HTTPSConfigPath)
	}

	if ha, ok := sanitized["highAvailability"].(bool); ok {
		input.Values.Set(settings.ConfigRoot+".highAvailability", ha)
	} else {
		input.Values.Remove(settings.ConfigRoot + ".highAvailability")
	}

	if controllerConfig := buildControllerConfig(sanitized); len(controllerConfig) > 0 {
		input.Values.Set(settings.InternalControllerPath+".config", controllerConfig)
	} else {
		input.Values.Remove(settings.InternalControllerPath + ".config")
	}

	return nil
}

func moduleConfigFromSnapshot(input *pkg.HookInput) (*moduleconfig.State, error) {
	snapshot := input.Snapshots.Get(moduleConfigSnapshot)
	if len(snapshot) == 0 {
		return nil, nil
	}

	var payload moduleConfigSnapshotPayload
	if err := snapshot[0].UnmarshalTo(&payload); err != nil {
		return nil, fmt.Errorf("decode ModuleConfig/%s: %w", settings.ModuleName, err)
	}

	state, err := moduleconfig.Parse(moduleconfig.Input{
		Enabled:  payload.Spec.Enabled,
		Settings: payload.Spec.Settings,
	})
	if err != nil {
		return nil, fmt.Errorf("parse ModuleConfig/%s: %w", settings.ModuleName, err)
	}

	clone := state.Clone()
	return &clone, nil
}

func registerValidationError(input *pkg.HookInput, err error) {
	input.Values.Set(settings.InternalModuleValidationPath, map[string]any{
		"error": err.Error(),
	})
}

func clearModuleValues(input *pkg.HookInput) {
	input.Values.Remove(settings.InternalModuleConfigPath)
	input.Values.Remove(settings.InternalControllerPath + ".config")
	input.Values.Remove(settings.HTTPSConfigPath)
}

func setValueOrRemove(input *pkg.HookInput, path string, value any) {
	if value == nil {
		input.Values.Remove(path)
		return
	}
	input.Values.Set(path, value)
}

func resolveHTTPSConfig(input *pkg.HookInput, user map[string]any) map[string]any {
	if user != nil {
		return user
	}

	mode := normalizeHTTPSMode(input.Values.Get("global.modules.https.mode").String())
	if mode == "" {
		mode = settings.DefaultHTTPSMode
	}

	if mode == "CustomCertificate" {
		secret := strings.TrimSpace(input.Values.Get("global.modules.https.customCertificate.secretName").String())
		if secret == "" {
			return nil
		}
		return map[string]any{
			"mode": "CustomCertificate",
			"customCertificate": map[string]any{
				"secretName": secret,
			},
		}
	}

	if mode == "OnlyInURI" || mode == "Disabled" {
		return map[string]any{"mode": mode}
	}

	issuer := strings.TrimSpace(input.Values.Get("global.modules.https.certManager.clusterIssuerName").String())
	if issuer == "" {
		issuer = settings.DefaultHTTPSClusterIssuer
	}
	return map[string]any{
		"mode": "CertManager",
		"certManager": map[string]any{
			"clusterIssuerName": issuer,
		},
	}
}

func buildControllerConfig(cfg map[string]any) map[string]any {
	if cfg == nil {
		return nil
	}

	result := make(map[string]any)

	if inventoryRaw, ok := cfg["inventory"].(map[string]any); ok {
		if period, ok := inventoryRaw["resyncPeriod"].(string); ok && strings.TrimSpace(period) != "" {
			result["controllers"] = map[string]any{
				"gpuInventory": map[string]any{
					"resyncPeriod": period,
				},
			}
		}
	}

	moduleSection := make(map[string]any)
	if managed, ok := cfg["managedNodes"]; ok {
		moduleSection["managedNodes"] = managed
	}
	if approval, ok := cfg["deviceApproval"]; ok {
		moduleSection["deviceApproval"] = approval
	}
	if scheduling, ok := cfg["scheduling"]; ok {
		moduleSection["scheduling"] = scheduling
	}
	if len(moduleSection) > 0 {
		result["module"] = moduleSection
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeHTTPSMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return ""
	case "certmanager":
		return "CertManager"
	case "customcertificate":
		return "CustomCertificate"
	case "onlyinuri":
		return "OnlyInURI"
	case "disabled":
		return "Disabled"
	default:
		return ""
	}
}
