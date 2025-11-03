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

package moduleconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Input struct {
	Enabled  *bool
	Settings map[string]any
	Global   GlobalValues
}

type GlobalValues struct {
	Mode              string
	CertManagerIssuer string
	CustomSecret      string
}

type State struct {
	Enabled          bool
	Settings         Settings
	Inventory        InventorySettings
	HighAvailability *bool
	HTTPS            HTTPSSettings
	Sanitized        map[string]any
}

type Settings struct {
	ManagedNodes   ManagedNodesSettings
	DeviceApproval DeviceApprovalSettings
	Scheduling     SchedulingSettings
}

type ManagedNodesSettings struct {
	LabelKey         string
	EnabledByDefault bool
}

type DeviceApprovalMode string

const (
	DeviceApprovalModeManual    DeviceApprovalMode = "Manual"
	DeviceApprovalModeAutomatic DeviceApprovalMode = "Automatic"
	DeviceApprovalModeSelector  DeviceApprovalMode = "Selector"
)

type DeviceApprovalSettings struct {
	Mode     DeviceApprovalMode
	Selector *metav1.LabelSelector
}

type SchedulingSettings struct {
	DefaultStrategy string
	TopologyKey     string
}

type InventorySettings struct {
	ResyncPeriod string
}

type HTTPSMode string

const (
	HTTPSModeDisabled          HTTPSMode = "Disabled"
	HTTPSModeCertManager       HTTPSMode = "CertManager"
	HTTPSModeCustomCertificate HTTPSMode = "CustomCertificate"
	HTTPSModeOnlyInURI         HTTPSMode = "OnlyInURI"
)

type HTTPSSettings struct {
	Mode                    HTTPSMode
	CertManagerIssuer       string
	CustomCertificateSecret string
}

const (
	DefaultNodeLabelKey           = "gpu.deckhouse.io/enabled"
	DefaultDeviceApprovalMode     = DeviceApprovalModeManual
	DefaultSchedulingStrategy     = "Spread"
	DefaultSchedulingTopology     = "topology.kubernetes.io/zone"
	DefaultInventoryResyncPeriod  = "30s"
	DefaultHTTPSMode              = HTTPSModeCertManager
	DefaultHTTPSCertManagerIssuer = "letsencrypt"
)

func DefaultState() State {
	settings := Settings{
		ManagedNodes:   ManagedNodesSettings{LabelKey: DefaultNodeLabelKey, EnabledByDefault: true},
		DeviceApproval: DeviceApprovalSettings{Mode: DeviceApprovalModeManual},
		Scheduling:     SchedulingSettings{DefaultStrategy: DefaultSchedulingStrategy, TopologyKey: DefaultSchedulingTopology},
	}
	sanitized := map[string]any{
		"managedNodes":   map[string]any{"labelKey": DefaultNodeLabelKey, "enabledByDefault": true},
		"deviceApproval": map[string]any{"mode": string(DefaultDeviceApprovalMode)},
		"scheduling":     map[string]any{"defaultStrategy": DefaultSchedulingStrategy, "topologyKey": DefaultSchedulingTopology},
		"inventory":      map[string]any{"resyncPeriod": DefaultInventoryResyncPeriod},
		"https":          map[string]any{"mode": string(DefaultHTTPSMode), "certManager": map[string]any{"clusterIssuerName": DefaultHTTPSCertManagerIssuer}},
	}
	return State{
		Settings:  settings,
		Inventory: InventorySettings{ResyncPeriod: DefaultInventoryResyncPeriod},
		HTTPS:     HTTPSSettings{Mode: DefaultHTTPSMode, CertManagerIssuer: DefaultHTTPSCertManagerIssuer},
		Sanitized: sanitized,
	}
}

func Parse(input Input) (State, error) {
	state := DefaultState()
	if input.Enabled != nil {
		state.Enabled = *input.Enabled
	}

	raw := make(map[string]json.RawMessage, len(input.Settings))
	for key, value := range input.Settings {
		data, err := json.Marshal(value)
		if err != nil {
			return state, fmt.Errorf("encode settings.%s: %w", key, err)
		}
		raw[key] = data
	}

	managed, err := parseManaged(raw["managedNodes"])
	if err != nil {
		return state, err
	}
	state.Settings.ManagedNodes = managed
	state.Sanitized["managedNodes"] = map[string]any{"labelKey": managed.LabelKey, "enabledByDefault": managed.EnabledByDefault}

	approval, selector, err := parseApproval(raw["deviceApproval"])
	if err != nil {
		return state, err
	}
	state.Settings.DeviceApproval = approval
	m := map[string]any{"mode": string(approval.Mode)}
	if selector != nil {
		m["selector"] = selector
	}
	state.Sanitized["deviceApproval"] = m

	scheduling, err := parseScheduling(raw["scheduling"])
	if err != nil {
		return state, err
	}
	state.Settings.Scheduling = scheduling
	state.Sanitized["scheduling"] = map[string]any{"defaultStrategy": scheduling.DefaultStrategy, "topologyKey": scheduling.TopologyKey}

	inventory, err := parseInventory(raw["inventory"])
	if err != nil {
		return state, err
	}
	state.Inventory = inventory
	state.Sanitized["inventory"] = map[string]any{"resyncPeriod": inventory.ResyncPeriod}

	https, httpsMap, err := parseHTTPS(raw["https"], input.Global)
	if err != nil {
		return state, err
	}
	state.HTTPS = https
	state.Sanitized["https"] = httpsMap

	if ha := parseBool(raw["highAvailability"]); ha != nil {
		state.HighAvailability = ha
		state.Sanitized["highAvailability"] = *ha
	}

	return state, nil
}

func (s State) Values() map[string]any {
	result := map[string]any{
		"managedNodes":   map[string]any{"labelKey": s.Settings.ManagedNodes.LabelKey, "enabledByDefault": s.Settings.ManagedNodes.EnabledByDefault},
		"deviceApproval": map[string]any{"mode": string(s.Settings.DeviceApproval.Mode)},
		"scheduling":     map[string]any{"defaultStrategy": s.Settings.Scheduling.DefaultStrategy},
		"inventory":      map[string]any{"resyncPeriod": s.Inventory.ResyncPeriod},
		"https":          map[string]any{"mode": string(s.HTTPS.Mode)},
		"internal":       map[string]any{"moduleConfig": map[string]any{"enabled": s.Enabled, "settings": s.Sanitized}},
	}
	if s.Settings.DeviceApproval.Selector != nil {
		result["deviceApproval"].(map[string]any)["selector"] = selectorToMap(*s.Settings.DeviceApproval.Selector)
	}
	if s.Settings.Scheduling.TopologyKey != "" {
		result["scheduling"].(map[string]any)["topologyKey"] = s.Settings.Scheduling.TopologyKey
	}
	switch s.HTTPS.Mode {
	case HTTPSModeCertManager:
		result["https"].(map[string]any)["certManager"] = map[string]any{"clusterIssuerName": s.HTTPS.CertManagerIssuer}
	case HTTPSModeCustomCertificate:
		result["https"].(map[string]any)["customCertificate"] = map[string]any{"secretName": s.HTTPS.CustomCertificateSecret}
	}
	if s.HighAvailability != nil {
		result["highAvailability"] = *s.HighAvailability
	}
	return result
}

func parseManaged(raw json.RawMessage) (ManagedNodesSettings, error) {
	settings := ManagedNodesSettings{LabelKey: DefaultNodeLabelKey, EnabledByDefault: true}
	if len(raw) == 0 || string(raw) == "null" {
		return settings, nil
	}
	var payload struct {
		LabelKey         string `json:"labelKey"`
		EnabledByDefault *bool  `json:"enabledByDefault"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return settings, fmt.Errorf("decode managedNodes: %w", err)
	}
	if v := strings.TrimSpace(payload.LabelKey); v != "" {
		settings.LabelKey = v
	}
	if payload.EnabledByDefault != nil {
		settings.EnabledByDefault = *payload.EnabledByDefault
	}
	return settings, nil
}

func parseApproval(raw json.RawMessage) (DeviceApprovalSettings, map[string]any, error) {
	settings := DeviceApprovalSettings{Mode: DeviceApprovalModeManual}
	if len(raw) == 0 || string(raw) == "null" {
		return settings, nil, nil
	}
	var payload struct {
		Mode     string          `json:"mode"`
		Selector json.RawMessage `json:"selector"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return settings, nil, fmt.Errorf("decode deviceApproval: %w", err)
	}
	if mode := normalizeApprovalMode(payload.Mode); mode != "" {
		settings.Mode = mode
	} else if strings.TrimSpace(payload.Mode) != "" {
		return settings, nil, fmt.Errorf("unknown deviceApproval.mode %q", payload.Mode)
	}
	var selector map[string]any
	if settings.Mode == DeviceApprovalModeSelector {
		if len(payload.Selector) == 0 || string(payload.Selector) == "null" {
			return settings, nil, nil
		}
		sel, mapped, err := parseSelector(payload.Selector)
		if err != nil {
			return settings, nil, err
		}
		settings.Selector = sel
		selector = mapped
	}
	return settings, selector, nil
}

func normalizeApprovalMode(mode string) DeviceApprovalMode {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return ""
	case "manual":
		return DeviceApprovalModeManual
	case "automatic":
		return DeviceApprovalModeAutomatic
	case "selector":
		return DeviceApprovalModeSelector
	default:
		return ""
	}
}

func parseSelector(raw json.RawMessage) (*metav1.LabelSelector, map[string]any, error) {
	var payload struct {
		MatchLabels      map[string]string `json:"matchLabels"`
		MatchExpressions []struct {
			Key      string   `json:"key"`
			Operator string   `json:"operator"`
			Values   []string `json:"values"`
		} `json:"matchExpressions"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("decode selector: %w", err)
	}
	selector := &metav1.LabelSelector{}
	mapped := make(map[string]any)
	if len(payload.MatchLabels) > 0 {
		labels := make(map[string]string)
		selector.MatchLabels = make(map[string]string)
		for key, value := range payload.MatchLabels {
			k := strings.TrimSpace(key)
			v := strings.TrimSpace(value)
			if k == "" || v == "" {
				return nil, nil, errors.New("deviceApproval.selector.matchLabels keys and values must be non-empty")
			}
			labels[k] = v
			selector.MatchLabels[k] = v
		}
		mapped["matchLabels"] = labels
	}
	if len(payload.MatchExpressions) > 0 {
		expr := make([]metav1.LabelSelectorRequirement, 0, len(payload.MatchExpressions))
		exprMap := make([]map[string]any, 0, len(payload.MatchExpressions))
		for _, item := range payload.MatchExpressions {
			op := normalizeSelectorOperator(item.Operator)
			if op == "" {
				return nil, nil, fmt.Errorf("unsupported selector operator %q", item.Operator)
			}
			key := strings.TrimSpace(item.Key)
			if key == "" {
				return nil, nil, errors.New("deviceApproval.selector.matchExpressions[].key must be set")
			}
			values := make([]string, 0, len(item.Values))
			for _, v := range item.Values {
				if val := strings.TrimSpace(v); val != "" {
					values = append(values, val)
				}
			}
			if (op == "In" || op == "NotIn") && len(values) == 0 {
				return nil, nil, fmt.Errorf("selector operator %q requires non-empty values", op)
			}
			if (op == "Exists" || op == "DoesNotExist") && len(values) > 0 {
				return nil, nil, fmt.Errorf("selector operator %q does not accept values", op)
			}
			expr = append(expr, metav1.LabelSelectorRequirement{
				Key:      key,
				Operator: metav1.LabelSelectorOperator(op),
				Values:   values,
			})
			exprMap = append(exprMap, map[string]any{
				"key":      key,
				"operator": op,
				"values":   values,
			})
		}
		selector.MatchExpressions = expr
		mapped["matchExpressions"] = exprMap
	}
	if selector.MatchLabels == nil && len(selector.MatchExpressions) == 0 {
		return nil, nil, errors.New("deviceApproval.selector must define matchLabels or matchExpressions")
	}
	return selector, mapped, nil
}

func normalizeSelectorOperator(op string) string {
	switch strings.ToLower(strings.TrimSpace(op)) {
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

func parseScheduling(raw json.RawMessage) (SchedulingSettings, error) {
	settings := SchedulingSettings{DefaultStrategy: DefaultSchedulingStrategy, TopologyKey: DefaultSchedulingTopology}
	if len(raw) == 0 || string(raw) == "null" {
		return settings, nil
	}
	var payload struct {
		DefaultStrategy string `json:"defaultStrategy"`
		TopologyKey     string `json:"topologyKey"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return settings, fmt.Errorf("decode scheduling settings: %w", err)
	}
	if strat := normalizeStrategy(payload.DefaultStrategy); strat != "" {
		settings.DefaultStrategy = strat
	} else if strings.TrimSpace(payload.DefaultStrategy) != "" {
		return settings, fmt.Errorf("unknown scheduling.defaultStrategy %q", payload.DefaultStrategy)
	}
	topo := strings.TrimSpace(payload.TopologyKey)
	if settings.DefaultStrategy == DefaultSchedulingStrategy && topo == "" {
		topo = DefaultSchedulingTopology
	}
	settings.TopologyKey = topo
	return settings, nil
}

func normalizeStrategy(strategy string) string {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "":
		return ""
	case "spread":
		return "Spread"
	case "binpack":
		return "BinPack"
	default:
		return ""
	}
}

func parseInventory(raw json.RawMessage) (InventorySettings, error) {
	settings := InventorySettings{ResyncPeriod: DefaultInventoryResyncPeriod}
	if len(raw) == 0 || string(raw) == "null" {
		return settings, nil
	}
	var payload struct {
		ResyncPeriod string `json:"resyncPeriod"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return settings, fmt.Errorf("decode inventory settings: %w", err)
	}
	if trimmed := strings.TrimSpace(payload.ResyncPeriod); trimmed != "" {
		if _, err := time.ParseDuration(trimmed); err != nil {
			return settings, fmt.Errorf("parse inventory.resyncPeriod: %w", err)
		}
		settings.ResyncPeriod = trimmed
	}
	return settings, nil
}

func parseHTTPS(raw json.RawMessage, globals GlobalValues) (HTTPSSettings, map[string]any, error) {
	mode := normalizeHTTPSMode(globals.Mode)
	if mode == "" {
		mode = DefaultHTTPSMode
	}
	issuer := strings.TrimSpace(globals.CertManagerIssuer)
	if issuer == "" {
		issuer = DefaultHTTPSCertManagerIssuer
	}
	secret := strings.TrimSpace(globals.CustomSecret)

	if len(raw) != 0 && string(raw) != "null" {
		var payload struct {
			Mode        string `json:"mode"`
			CertManager struct {
				ClusterIssuerName string `json:"clusterIssuerName"`
			} `json:"certManager"`
			CustomCertificate struct {
				SecretName string `json:"secretName"`
			} `json:"customCertificate"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return HTTPSSettings{}, nil, fmt.Errorf("decode https settings: %w", err)
		}
		if v := normalizeHTTPSMode(payload.Mode); v != "" {
			mode = v
		} else if strings.TrimSpace(payload.Mode) != "" {
			return HTTPSSettings{}, nil, fmt.Errorf("unknown https.mode %q", payload.Mode)
		}
		if m := strings.TrimSpace(payload.CertManager.ClusterIssuerName); m != "" {
			issuer = m
		}
		if s := strings.TrimSpace(payload.CustomCertificate.SecretName); s != "" {
			secret = s
		}
	}

	switch mode {
	case HTTPSModeCustomCertificate:
		if secret == "" {
			return HTTPSSettings{}, nil, errors.New("https.customCertificate.secretName must be set when mode=CustomCertificate")
		}
		return HTTPSSettings{Mode: mode, CustomCertificateSecret: secret}, map[string]any{
			"mode":              string(mode),
			"customCertificate": map[string]any{"secretName": secret},
		}, nil
	case HTTPSModeOnlyInURI, HTTPSModeDisabled:
		return HTTPSSettings{Mode: mode}, map[string]any{"mode": string(mode)}, nil
	default:
		return HTTPSSettings{Mode: HTTPSModeCertManager, CertManagerIssuer: issuer}, map[string]any{
			"mode":        string(HTTPSModeCertManager),
			"certManager": map[string]any{"clusterIssuerName": issuer},
		}, nil
	}
}

func normalizeHTTPSMode(mode string) HTTPSMode {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return ""
	case "certmanager":
		return HTTPSModeCertManager
	case "customcertificate":
		return HTTPSModeCustomCertificate
	case "onlyinuri":
		return HTTPSModeOnlyInURI
	case "disabled":
		return HTTPSModeDisabled
	default:
		return ""
	}
}

func parseBool(raw json.RawMessage) *bool {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var flag bool
	if err := json.Unmarshal(raw, &flag); err != nil {
		return nil
	}
	return &flag
}

// Clone performs a deep copy of the state to guarantee isolation between store consumers.
func (s State) Clone() State {
	clone := s
	if s.Settings.DeviceApproval.Selector != nil {
		clone.Settings.DeviceApproval.Selector = s.Settings.DeviceApproval.Selector.DeepCopy()
	}
	clone.Sanitized = deepCopySanitizedMap(s.Sanitized)
	return clone
}

func deepCopySanitizedMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = deepCopyValue(value)
	}
	return dst
}

func deepCopyValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return deepCopySanitizedMap(v)
	case map[string]string:
		out := make(map[string]string, len(v))
		for key, val := range v {
			out[key] = val
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = deepCopyValue(item)
		}
		return out
	case []string:
		out := make([]string, len(v))
		copy(out, v)
		return out
	default:
		return v
	}
}

func selectorToMap(selector metav1.LabelSelector) map[string]any {
	result := make(map[string]any)
	if len(selector.MatchLabels) > 0 {
		labels := make(map[string]string)
		for k, v := range selector.MatchLabels {
			labels[k] = v
		}
		result["matchLabels"] = labels
	}
	if len(selector.MatchExpressions) > 0 {
		expr := make([]map[string]any, 0, len(selector.MatchExpressions))
		for _, item := range selector.MatchExpressions {
			expr = append(expr, map[string]any{
				"key":      item.Key,
				"operator": string(item.Operator),
				"values":   append([]string(nil), item.Values...),
			})
		}
		result["matchExpressions"] = expr
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
