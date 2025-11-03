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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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

type moduleConfigState struct {
	Enabled bool
	Config  map[string]any
}

var moduleConfigFromSnapshotFn = moduleConfigFromSnapshot

var (
	defaultSchedulingTopology    = settings.DefaultSchedulingTopology
	defaultInventoryResyncPeriod = settings.DefaultInventoryResyncPeriod
	jsonUnmarshal                = json.Unmarshal
)

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
		input.Values.Remove(settings.InternalModuleConfigPath)
		input.Values.Remove(settings.InternalControllerPath + ".config")
		input.Values.Remove(settings.HTTPSConfigPath)
		return nil
	}

	if state == nil {
		input.Values.Remove(settings.InternalModuleConfigPath)
		input.Values.Remove(settings.InternalModuleValidationPath)
		input.Values.Remove(settings.InternalControllerPath + ".config")
		input.Values.Remove(settings.HTTPSConfigPath)
		return nil
	}

	input.Values.Remove(settings.InternalModuleValidationPath)
	payload := map[string]any{
		"enabled": state.Enabled,
	}
	if len(state.Config) > 0 {
		payload["settings"] = state.Config
	}
	input.Values.Set(settings.InternalModuleConfigPath, payload)

	if managed, ok := state.Config["managedNodes"]; ok {
		input.Values.Set(settings.ConfigRoot+".managedNodes", managed)
	} else {
		input.Values.Remove(settings.ConfigRoot + ".managedNodes")
	}

	if approval, ok := state.Config["deviceApproval"]; ok {
		input.Values.Set(settings.ConfigRoot+".deviceApproval", approval)
	} else {
		input.Values.Remove(settings.ConfigRoot + ".deviceApproval")
	}

	if scheduling, ok := state.Config["scheduling"]; ok {
		input.Values.Set(settings.ConfigRoot+".scheduling", scheduling)
	} else {
		input.Values.Remove(settings.ConfigRoot + ".scheduling")
	}

	if inventory, ok := state.Config["inventory"]; ok {
		input.Values.Set(settings.ConfigRoot+".inventory", inventory)
	} else {
		input.Values.Remove(settings.ConfigRoot + ".inventory")
	}

	var userHTTPS map[string]any
	if httpsCfg, ok := state.Config["https"]; ok {
		if cast, ok := httpsCfg.(map[string]any); ok {
			userHTTPS = cast
		}
	}

	effectiveHTTPS := resolveHTTPSConfig(input, userHTTPS)
	if effectiveHTTPS != nil {
		input.Values.Set(settings.HTTPSConfigPath, effectiveHTTPS)
	} else {
		input.Values.Remove(settings.HTTPSConfigPath)
	}

	if haRaw, ok := state.Config["highAvailability"]; ok {
		if flag, ok := haRaw.(bool); ok {
			input.Values.Set(settings.ConfigRoot+".highAvailability", flag)
		} else {
			input.Values.Remove(settings.ConfigRoot + ".highAvailability")
		}
	} else {
		input.Values.Remove(settings.ConfigRoot + ".highAvailability")
	}

	controllerConfig := buildControllerConfig(state.Config)
	if len(controllerConfig) > 0 {
		input.Values.Set(settings.InternalControllerPath+".config", controllerConfig)
	} else {
		input.Values.Remove(settings.InternalControllerPath + ".config")
	}

	return nil
}

func moduleConfigFromSnapshot(input *pkg.HookInput) (*moduleConfigState, error) {
	snapshot := input.Snapshots.Get(moduleConfigSnapshot)
	if len(snapshot) == 0 {
		return nil, nil
	}

	var mc moduleConfigSnapshotPayload
	if err := snapshot[0].UnmarshalTo(&mc); err != nil {
		return nil, fmt.Errorf("decode ModuleConfig/%s: %w", settings.ModuleName, err)
	}

	cfg, err := sanitizeModuleSettings(mc.Spec.Settings)
	if err != nil {
		return nil, err
	}

	enabled := false
	if mc.Spec.Enabled != nil {
		enabled = *mc.Spec.Enabled
	}

	return &moduleConfigState{
		Enabled: enabled,
		Config:  cfg,
	}, nil
}

func registerValidationError(input *pkg.HookInput, err error) {
	input.Values.Set(settings.InternalModuleValidationPath, map[string]any{
		"error": err.Error(),
	})
}

func sanitizeModuleSettings(raw map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	var asJSON []byte
	var err error
	if raw != nil {
		asJSON, err = json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("encode ModuleConfig settings: %w", err)
		}
	}

	fields := make(map[string]json.RawMessage)
	if len(asJSON) > 0 {
		if err := jsonUnmarshal(asJSON, &fields); err != nil {
			return nil, fmt.Errorf("decode ModuleConfig settings: %w", err)
		}
	}

	managed, err := sanitizeManagedNodes(fields["managedNodes"])
	if err != nil {
		return nil, err
	}
	result["managedNodes"] = managed

	approval, err := sanitizeDeviceApproval(fields["deviceApproval"])
	if err != nil {
		return nil, err
	}
	result["deviceApproval"] = approval

	scheduling, err := sanitizeScheduling(fields["scheduling"])
	if err != nil {
		return nil, err
	}
	result["scheduling"] = scheduling

	inventory, err := sanitizeInventory(fields["inventory"])
	if err != nil {
		return nil, err
	}
	result["inventory"] = inventory

	if rawHTTPS := fields["https"]; len(rawHTTPS) > 0 && string(rawHTTPS) != "null" {
		https, err := sanitizeHTTPS(rawHTTPS)
		if err != nil {
			return nil, err
		}
		result["https"] = https
	}

	if rawHA, ok := fields["highAvailability"]; ok {
		if len(rawHA) != 0 && string(rawHA) != "null" {
			var enabled bool
			if err := jsonUnmarshal(rawHA, &enabled); err != nil {
				return nil, fmt.Errorf("decode ModuleConfig field %q: %w", "highAvailability", err)
			}
			result["highAvailability"] = enabled
		}
	}

	return result, nil
}

type rawManagedNodes struct {
	LabelKey         string `json:"labelKey"`
	EnabledByDefault *bool  `json:"enabledByDefault"`
}

type rawDeviceApproval struct {
	Mode     string         `json:"mode"`
	Selector *labelSelector `json:"selector"`
}

type rawScheduling struct {
	DefaultStrategy string `json:"defaultStrategy"`
	TopologyKey     string `json:"topologyKey"`
}

type rawInventory struct {
	ResyncPeriod string `json:"resyncPeriod"`
}

type rawHTTPS struct {
	Mode              string                     `json:"mode"`
	CertManager       *rawHTTPSCertManager       `json:"certManager"`
	CustomCertificate *rawHTTPSCustomCertificate `json:"customCertificate"`
}

type rawHTTPSCertManager struct {
	ClusterIssuerName string `json:"clusterIssuerName"`
}

type rawHTTPSCustomCertificate struct {
	SecretName string `json:"secretName"`
}

type labelSelector struct {
	MatchLabels      map[string]string   `json:"matchLabels"`
	MatchExpressions []labelSelectorRule `json:"matchExpressions"`
}

type labelSelectorRule struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
}

func sanitizeManagedNodes(raw json.RawMessage) (map[string]any, error) {
	label := settings.DefaultNodeLabelKey
	enabled := true

	if len(raw) > 0 && string(raw) != "null" {
		var payload rawManagedNodes
		if err := jsonUnmarshal(raw, &payload); err != nil {
			return nil, fmt.Errorf("decode managedNodes settings: %w", err)
		}
		if v := strings.TrimSpace(payload.LabelKey); v != "" {
			label = v
		}
		if payload.EnabledByDefault != nil {
			enabled = *payload.EnabledByDefault
		}
	}

	return map[string]any{
		"labelKey":         label,
		"enabledByDefault": enabled,
	}, nil
}

func sanitizeDeviceApproval(raw json.RawMessage) (map[string]any, error) {
	mode := settings.DefaultAutoAssignmentMode
	var selector map[string]any

	if len(raw) > 0 && string(raw) != "null" {
		var payload rawDeviceApproval
		if err := jsonUnmarshal(raw, &payload); err != nil {
			return nil, fmt.Errorf("decode deviceApproval settings: %w", err)
		}
		if autoMode := normalizeMode(payload.Mode); autoMode != "" {
			mode = autoMode
		} else if strings.TrimSpace(payload.Mode) != "" {
			return nil, fmt.Errorf("unknown deviceApproval.mode %q", payload.Mode)
		}

		if mode == "Selector" {
			if payload.Selector == nil {
				return nil, errors.New("deviceApproval.selector must be set when mode=Selector")
			}
			sel, err := sanitizeSelector(payload.Selector)
			if err != nil {
				return nil, err
			}
			selector = sel
		}
	}

	result := map[string]any{
		"mode": mode,
	}
	if selector != nil {
		result["selector"] = selector
	}
	return result, nil
}

func sanitizeScheduling(raw json.RawMessage) (map[string]any, error) {
	strategy := settings.DefaultSchedulingStrategy
	topology := defaultSchedulingTopology

	if len(raw) > 0 && string(raw) != "null" {
		var payload rawScheduling
		if err := jsonUnmarshal(raw, &payload); err != nil {
			return nil, fmt.Errorf("decode scheduling settings: %w", err)
		}
		if v := normalizeStrategy(payload.DefaultStrategy); v != "" {
			strategy = v
		} else if strings.TrimSpace(payload.DefaultStrategy) != "" {
			return nil, fmt.Errorf("unknown scheduling.defaultStrategy %q", payload.DefaultStrategy)
		}
		if payload.TopologyKey != "" {
			topology = strings.TrimSpace(payload.TopologyKey)
		}
	}

	if strategy == "Spread" && topology == "" {
		topology = defaultSchedulingTopology
	}
	if strategy == "Spread" && strings.TrimSpace(topology) == "" {
		return nil, errors.New("scheduling.topologyKey must be set when defaultStrategy=Spread")
	}

	result := map[string]any{
		"defaultStrategy": strategy,
	}
	if topology != "" {
		result["topologyKey"] = topology
	}
	return result, nil
}

func sanitizeInventory(raw json.RawMessage) (map[string]any, error) {
	period := defaultInventoryResyncPeriod

	if len(raw) > 0 && string(raw) != "null" {
		var payload rawInventory
		if err := jsonUnmarshal(raw, &payload); err != nil {
			return nil, fmt.Errorf("decode inventory settings: %w", err)
		}
		if trimmed := strings.TrimSpace(payload.ResyncPeriod); trimmed != "" {
			if _, err := time.ParseDuration(trimmed); err != nil {
				return nil, fmt.Errorf("parse inventory.resyncPeriod: %w", err)
			}
			period = trimmed
		}
	}

	if _, err := time.ParseDuration(period); err != nil {
		return nil, fmt.Errorf("parse inventory.resyncPeriod: %w", err)
	}

	return map[string]any{
		"resyncPeriod": period,
	}, nil
}

func sanitizeHTTPS(raw json.RawMessage) (map[string]any, error) {
	var payload rawHTTPS
	if err := jsonUnmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode https settings: %w", err)
	}

	mode := normalizeHTTPSMode(payload.Mode)
	if mode == "" {
		if strings.TrimSpace(payload.Mode) != "" {
			return nil, fmt.Errorf("unknown https.mode %q", payload.Mode)
		}
		mode = settings.DefaultHTTPSMode
	}

	result := map[string]any{"mode": mode}

	switch mode {
	case "CertManager":
		issuer := settings.DefaultHTTPSClusterIssuer
		if payload.CertManager != nil {
			if trimmed := strings.TrimSpace(payload.CertManager.ClusterIssuerName); trimmed != "" {
				issuer = trimmed
			}
		}
		result["certManager"] = map[string]any{"clusterIssuerName": issuer}
	case "CustomCertificate":
		if payload.CustomCertificate == nil {
			return nil, errors.New("https.customCertificate.secretName must be set when mode=CustomCertificate")
		}
		secret := strings.TrimSpace(payload.CustomCertificate.SecretName)
		if secret == "" {
			return nil, errors.New("https.customCertificate.secretName must be set when mode=CustomCertificate")
		}
		result["customCertificate"] = map[string]any{"secretName": secret}
	case "OnlyInURI", "Disabled":
		// no additional sections required
	}

	return result, nil
}

func sanitizeSelector(sel *labelSelector) (map[string]any, error) {
	if sel == nil {
		return nil, errors.New("deviceApproval.selector cannot be null")
	}

	matchLabels := make(map[string]string)
	for key, value := range sel.MatchLabels {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return nil, errors.New("deviceApproval.selector.matchLabels keys and values must be non-empty")
		}
		matchLabels[key] = value
	}

	var matchExpressions []map[string]any
	for _, req := range sel.MatchExpressions {
		op := strings.TrimSpace(req.Operator)
		if op == "" {
			return nil, errors.New("deviceApproval.selector.matchExpressions[].operator must be set")
		}
		op = normalizeOperator(op)
		if op == "" {
			return nil, fmt.Errorf("unsupported selector operator %q", req.Operator)
		}

		key := strings.TrimSpace(req.Key)
		if key == "" {
			return nil, errors.New("deviceApproval.selector.matchExpressions[].key must be set")
		}

		values := make([]string, 0, len(req.Values))
		for _, v := range req.Values {
			v = strings.TrimSpace(v)
			if v != "" {
				values = append(values, v)
			}
		}

		if (op == "In" || op == "NotIn") && len(values) == 0 {
			return nil, fmt.Errorf("selector operator %q requires non-empty values", op)
		}
		if (op == "Exists" || op == "DoesNotExist") && len(values) > 0 {
			return nil, fmt.Errorf("selector operator %q does not accept values", op)
		}

		matchExpressions = append(matchExpressions, map[string]any{
			"key":      key,
			"operator": op,
			"values":   values,
		})
	}

	if len(matchLabels) == 0 && len(matchExpressions) == 0 {
		return nil, errors.New("deviceApproval.selector must define matchLabels or matchExpressions")
	}

	result := make(map[string]any)
	if len(matchLabels) > 0 {
		result["matchLabels"] = matchLabels
	}
	if len(matchExpressions) > 0 {
		result["matchExpressions"] = matchExpressions
	}
	return result, nil
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

func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return ""
	case "manual":
		return "Manual"
	case "automatic":
		return "Automatic"
	case "selector":
		return "Selector"
	default:
		return ""
	}
}

func normalizeStrategy(strategy string) string {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "":
		return ""
	case "binpack":
		return "BinPack"
	case "spread":
		return "Spread"
	default:
		return ""
	}
}

func normalizeOperator(op string) string {
	switch strings.ToLower(op) {
	case "in":
		return "In"
	case "notin":
		return "NotIn"
	case "exists":
		return "Exists"
	case "doesnotexist":
		return "DoesNotExist"
	default:
		return ""
	}
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
