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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type marshalError struct{}

func (marshalError) MarshalJSON() ([]byte, error) { return nil, errors.New("marshal error") }

func boolPtr(v bool) *bool { return &v }

func TestParse(t *testing.T) {
	cases := []struct {
		name  string
		input Input
		check func(*testing.T, State)
	}{
		{
			name:  "defaults",
			input: Input{},
			check: func(t *testing.T, got State) {
				if got.Enabled {
					t.Fatalf("expected enabled=false")
				}
				if got.Settings.ManagedNodes.LabelKey != DefaultNodeLabelKey || !got.Settings.ManagedNodes.EnabledByDefault {
					t.Fatalf("unexpected managed defaults: %+v", got.Settings.ManagedNodes)
				}
				if got.Settings.DeviceApproval.Mode != DefaultDeviceApprovalMode {
					t.Fatalf("unexpected approval mode: %s", got.Settings.DeviceApproval.Mode)
				}
				if got.Settings.Scheduling.DefaultStrategy != DefaultSchedulingStrategy || got.Settings.Scheduling.TopologyKey != DefaultSchedulingTopology {
					t.Fatalf("unexpected scheduling defaults: %+v", got.Settings.Scheduling)
				}
				if got.Inventory.ResyncPeriod != DefaultInventoryResyncPeriod {
					t.Fatalf("unexpected inventory default: %s", got.Inventory.ResyncPeriod)
				}
				if !got.Settings.Monitoring.ServiceMonitor {
					t.Fatalf("expected monitoring serviceMonitor default true")
				}
				if got.HighAvailability != nil {
					t.Fatalf("expected highAvailability=nil")
				}
				if got.HTTPS.Mode != DefaultHTTPSMode || got.HTTPS.CertManagerIssuer != DefaultHTTPSCertManagerIssuer {
					t.Fatalf("unexpected HTTPS defaults: %+v", got.HTTPS)
				}
			},
		},
		{
			name: "custom settings",
			input: Input{
				Enabled: boolPtr(true),
				Settings: map[string]any{
					"managedNodes": map[string]any{"labelKey": " custom ", "enabledByDefault": false},
					"deviceApproval": map[string]any{
						"mode": "Selector",
						"selector": map[string]any{
							"matchLabels": map[string]any{"gpu.deckhouse.io/model": "A100"},
						},
					},
					"scheduling": map[string]any{"defaultStrategy": "BinPack", "topologyKey": " zone "},
					"monitoring": map[string]any{"serviceMonitor": false},
					"inventory":  map[string]any{"resyncPeriod": "45s"},
					"https": map[string]any{
						"mode":              "CustomCertificate",
						"customCertificate": map[string]any{"secretName": "corp-secret"},
					},
					"highAvailability": true,
				},
			},
			check: func(t *testing.T, got State) {
				if !got.Enabled {
					t.Fatalf("expected enabled true")
				}
				if got.Settings.ManagedNodes.LabelKey != "custom" || got.Settings.ManagedNodes.EnabledByDefault {
					t.Fatalf("unexpected managed nodes: %+v", got.Settings.ManagedNodes)
				}
				if got.Settings.DeviceApproval.Mode != DeviceApprovalModeSelector || got.Settings.DeviceApproval.Selector == nil {
					t.Fatalf("expected selector approval, got %+v", got.Settings.DeviceApproval)
				}
				if got.Settings.Scheduling.DefaultStrategy != "BinPack" || got.Settings.Scheduling.TopologyKey != "zone" {
					t.Fatalf("unexpected scheduling: %+v", got.Settings.Scheduling)
				}
				if got.Inventory.ResyncPeriod != "45s" {
					t.Fatalf("unexpected inventory resync: %s", got.Inventory.ResyncPeriod)
				}
				if got.Settings.Monitoring.ServiceMonitor {
					t.Fatalf("expected monitoring serviceMonitor to be false")
				}
				if got.HTTPS.Mode != HTTPSModeCustomCertificate || got.HTTPS.CustomCertificateSecret != "corp-secret" {
					t.Fatalf("unexpected HTTPS settings: %+v", got.HTTPS)
				}
				if got.HighAvailability == nil || !*got.HighAvailability {
					t.Fatalf("expected highAvailability true")
				}
			},
		},
		{
			name:  "globals override",
			input: Input{Global: GlobalValues{Mode: "CustomCertificate", CustomSecret: "global-secret"}},
			check: func(t *testing.T, got State) {
				if got.HTTPS.Mode != HTTPSModeCustomCertificate || got.HTTPS.CustomCertificateSecret != "global-secret" {
					t.Fatalf("unexpected HTTPS from globals: %+v", got.HTTPS)
				}
			},
		},
		{
			name:  "null inventory",
			input: Input{Settings: map[string]any{"inventory": nil}},
			check: func(t *testing.T, got State) {
				if got.Inventory.ResyncPeriod != DefaultInventoryResyncPeriod {
					t.Fatalf("expected default inventory period")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			tc.check(t, state)
		})
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name    string
		input   Input
		wantErr string
	}{
		{"encode settings", Input{Settings: map[string]any{"invalid": marshalError{}}}, "encode settings.invalid"},
		{"managed nodes", Input{Settings: map[string]any{"managedNodes": map[string]any{"enabledByDefault": "oops"}}}, "decode managedNodes"},
		{"unknown approval mode", Input{Settings: map[string]any{"deviceApproval": map[string]any{"mode": "unsupported"}}}, "unknown deviceApproval.mode"},
		{"selector error", Input{Settings: map[string]any{"deviceApproval": map[string]any{"mode": "Selector", "selector": map[string]any{"matchLabels": map[string]any{"": "value"}}}}}, "matchLabels"},
		{"scheduling error", Input{Settings: map[string]any{"scheduling": map[string]any{"defaultStrategy": "invalid"}}}, "unknown scheduling"},
		{"monitoring decode", Input{Settings: map[string]any{"monitoring": "oops"}}, "decode monitoring"},
		{"inventory decode", Input{Settings: map[string]any{"inventory": "oops"}}, "decode inventory settings"},
		{"inventory error", Input{Settings: map[string]any{"inventory": map[string]any{"resyncPeriod": "bad"}}}, "parse inventory"},
		{"inventory duration overflow", Input{Settings: map[string]any{"inventory": map[string]any{"resyncPeriod": "9223372036854775808h"}}}, "parse inventory"},
		{"https decode", Input{Settings: map[string]any{"https": "oops"}}, "decode https settings"},
		{"https unknown mode", Input{Settings: map[string]any{"https": map[string]any{"mode": "unsupported"}}}, "unknown https.mode"},
		{"https custom certificate missing secret", Input{Settings: map[string]any{"https": map[string]any{"mode": "CustomCertificate"}}}, "secretName must be set"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestValues(t *testing.T) {
	state := State{
		Enabled: true,
		Settings: Settings{
			ManagedNodes: ManagedNodesSettings{LabelKey: "custom", EnabledByDefault: false},
			DeviceApproval: DeviceApprovalSettings{
				Mode:     DeviceApprovalModeSelector,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"gpu": "true"}},
			},
			Scheduling: SchedulingSettings{DefaultStrategy: "BinPack", TopologyKey: "zone"},
			Monitoring: MonitoringSettings{ServiceMonitor: false},
		},
		Inventory:        InventorySettings{ResyncPeriod: "45s"},
		HTTPS:            HTTPSSettings{Mode: HTTPSModeCustomCertificate, CustomCertificateSecret: "secret"},
		HighAvailability: boolPtr(true),
		Sanitized:        map[string]any{"managedNodes": map[string]any{}},
	}

	values := state.Values()
	https := values["https"].(map[string]any)
	if https["customCertificate"].(map[string]any)["secretName"].(string) != "secret" {
		t.Fatalf("expected secret in values")
	}
	if !values["highAvailability"].(bool) {
		t.Fatalf("expected highAvailability flag")
	}
	if monitor := values["monitoring"].(map[string]any)["serviceMonitor"].(bool); monitor {
		t.Fatalf("expected serviceMonitor value propagated")
	}

	state.HTTPS = HTTPSSettings{Mode: HTTPSModeCertManager, CertManagerIssuer: "issuer"}
	values = state.Values()
	issuer := values["https"].(map[string]any)["certManager"].(map[string]any)["clusterIssuerName"].(string)
	if issuer != "issuer" {
		t.Fatalf("expected cert manager issuer in values")
	}
}

func TestCloneDeepCopy(t *testing.T) {
	state := State{
		Settings: Settings{
			ManagedNodes: ManagedNodesSettings{LabelKey: "custom", EnabledByDefault: true},
			DeviceApproval: DeviceApprovalSettings{
				Mode:     DeviceApprovalModeSelector,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"gpu": "true"}},
			},
		},
		Sanitized: map[string]any{
			"managedNodes": map[string]any{"labelKey": "custom"},
			"array":        []any{"a", "b"},
		},
	}

	clone := state.Clone()
	clone.Settings.ManagedNodes.LabelKey = "changed"
	clone.Settings.DeviceApproval.Selector.MatchLabels["gpu"] = "false"
	clone.Sanitized["managedNodes"].(map[string]any)["labelKey"] = "changed"
	clone.Sanitized["array"].([]any)[0] = "x"

	if state.Settings.ManagedNodes.LabelKey != "custom" {
		t.Fatalf("managed nodes modified in original")
	}
	if state.Settings.DeviceApproval.Selector.MatchLabels["gpu"] != "true" {
		t.Fatalf("selector modified in original")
	}
	if state.Sanitized["managedNodes"].(map[string]any)["labelKey"].(string) != "custom" {
		t.Fatalf("sanitized map modified in original")
	}
	if state.Sanitized["array"].([]any)[0].(string) != "a" {
		t.Fatalf("slice modified in original")
	}
}

func TestParsePlacement(t *testing.T) {
	settings := parsePlacement(nil)
	if settings.CustomTolerationKeys == nil || len(settings.CustomTolerationKeys) != 0 {
		t.Fatalf("expected default empty slice, got %#v", settings.CustomTolerationKeys)
	}

	settings = parsePlacement(json.RawMessage(`"oops"`))
	if settings.CustomTolerationKeys == nil || len(settings.CustomTolerationKeys) != 0 {
		t.Fatalf("expected defaults for invalid JSON, got %#v", settings.CustomTolerationKeys)
	}

	settings = parsePlacement(json.RawMessage(`{"customTolerationKeys":[" a ","b","b",""," a "],"extra":"ignored"}`))
	if got, want := settings.CustomTolerationKeys, []string{"a", "b"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("unexpected toleration keys: %#v", got)
	}
}

func TestParseSelector(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "valid", raw: `{"matchLabels":{"gpu":"true"},"matchExpressions":[{"key":"class","operator":"NotIn","values":["low"]}]}`},
		{name: "valid mixed operators", raw: `{"matchExpressions":[{"key":"exists","operator":"Exists"},{"key":"missing","operator":"DoesNotExist"}]}`},
		{name: "valid in operator", raw: `{"matchExpressions":[{"key":"gpu","operator":"In","values":["  a ","b"]}]}`},
		{name: "operator does not exist", raw: `{"matchExpressions":[{"key":"gpu","operator":"DoesNotExist"}]}`},
		{name: "decode error", raw: `"invalid"`, wantErr: "decode selector"},
		{name: "empty label key", raw: `{"matchLabels":{" ":"value"}}`, wantErr: "matchLabels"},
		{name: "empty label value", raw: `{"matchLabels":{"key":" "}}`, wantErr: "matchLabels"},
		{name: "missing values for In", raw: `{"matchExpressions":[{"key":"gpu","operator":"In"}]}`, wantErr: "requires non-empty values"},
		{name: "missing values for NotIn", raw: `{"matchExpressions":[{"key":"gpu","operator":"NotIn"}]}`, wantErr: "requires non-empty values"},
		{name: "values with Exists", raw: `{"matchExpressions":[{"key":"gpu","operator":"Exists","values":["x"]}]}`, wantErr: "does not accept values"},
		{name: "unsupported operator", raw: `{"matchExpressions":[{"key":"gpu","operator":"Unsupported"}]}`, wantErr: "unsupported selector operator"},
		{name: "empty selector", raw: `{"matchLabels":{},"matchExpressions":[]}`, wantErr: "must define"},
		{name: "empty match expression key", raw: `{"matchExpressions":[{"key":" ","operator":"In","values":["gpu"]}]}`, wantErr: "must be set"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			selector, mapped, err := parseSelector(json.RawMessage(tc.raw))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if selector == nil || mapped == nil {
				t.Fatalf("expected selector and mapped output")
			}
		})
	}
}

func TestNormalizeHelpers(t *testing.T) {
	if normalizeStrategy("binpack") != "BinPack" || normalizeStrategy("spread") != "Spread" || normalizeStrategy("") != "" || normalizeStrategy("other") != "" {
		t.Fatalf("normalizeStrategy unexpected result")
	}
	if normalizeApprovalMode("Selector") != DeviceApprovalModeSelector || normalizeApprovalMode("Automatic") != DeviceApprovalModeAutomatic || normalizeApprovalMode("Manual") != DeviceApprovalModeManual || normalizeApprovalMode("") != "" || normalizeApprovalMode("other") != "" {
		t.Fatalf("normalizeApprovalMode unexpected result")
	}
	if normalizeHTTPSMode("Disabled") != HTTPSModeDisabled || normalizeHTTPSMode("OnlyInURI") != HTTPSModeOnlyInURI || normalizeHTTPSMode("") != "" || normalizeHTTPSMode("other") != "" {
		t.Fatalf("normalizeHTTPSMode unexpected result")
	}
	if normalizeSelectorOperator("in") != "In" || normalizeSelectorOperator("NOTIN") != "NotIn" || normalizeSelectorOperator("exists") != "Exists" || normalizeSelectorOperator("doesNotExist") != "DoesNotExist" || normalizeSelectorOperator("bad") != "" {
		t.Fatalf("normalizeSelectorOperator unexpected result")
	}
}

func TestParseScheduling(t *testing.T) {
	cases := []struct {
		name    string
		raw     json.RawMessage
		expect  SchedulingSettings
		wantErr string
	}{
		{name: "defaults", expect: SchedulingSettings{DefaultStrategy: DefaultSchedulingStrategy, TopologyKey: DefaultSchedulingTopology}},
		{name: "null raw", raw: json.RawMessage("null"), expect: SchedulingSettings{DefaultStrategy: DefaultSchedulingStrategy, TopologyKey: DefaultSchedulingTopology}},
		{name: "spread with blank topology", raw: json.RawMessage(`{"defaultStrategy":"Spread","topologyKey":"   "}`), expect: SchedulingSettings{DefaultStrategy: "Spread", TopologyKey: DefaultSchedulingTopology}},
		{name: "binpack trims topology", raw: json.RawMessage(`{"defaultStrategy":"BinPack","topologyKey":" zone "}`), expect: SchedulingSettings{DefaultStrategy: "BinPack", TopologyKey: "zone"}},
		{name: "unknown strategy", raw: json.RawMessage(`{"defaultStrategy":"invalid"}`), wantErr: "unknown scheduling"},
		{name: "decode error", raw: json.RawMessage(`"oops"`), wantErr: "decode scheduling"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseScheduling(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expect {
				t.Fatalf("unexpected scheduling: %+v", got)
			}
		})
	}
}

func TestParseInventory(t *testing.T) {
	cases := []struct {
		name    string
		raw     json.RawMessage
		expect  string
		wantErr string
	}{
		{name: "defaults", expect: DefaultInventoryResyncPeriod},
		{name: "null string", raw: json.RawMessage("null"), expect: DefaultInventoryResyncPeriod},
		{name: "trimmed duration", raw: json.RawMessage(`{"resyncPeriod":" 60s "}`), expect: "60s"},
		{name: "blank duration", raw: json.RawMessage(`{"resyncPeriod":"   "}`), expect: DefaultInventoryResyncPeriod},
		{name: "unsupported unit", raw: json.RawMessage(`{"resyncPeriod":"500ms"}`), wantErr: "does not match"},
		{name: "invalid duration", raw: json.RawMessage(`{"resyncPeriod":"bad"}`), wantErr: "parse inventory"},
		{name: "decode error", raw: json.RawMessage(`"oops"`), wantErr: "decode inventory"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseInventory(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ResyncPeriod != tc.expect {
				t.Fatalf("unexpected resync period: %s", got.ResyncPeriod)
			}
		})
	}
}

func TestParseHTTPS(t *testing.T) {
	cases := []struct {
		name    string
		raw     json.RawMessage
		globals GlobalValues
		expect  HTTPSSettings
		wantErr string
	}{
		{name: "defaults", expect: HTTPSSettings{Mode: HTTPSModeCertManager, CertManagerIssuer: DefaultHTTPSCertManagerIssuer}},
		{name: "null raw", raw: json.RawMessage("null"), expect: HTTPSSettings{Mode: HTTPSModeCertManager, CertManagerIssuer: DefaultHTTPSCertManagerIssuer}},
		{name: "disabled", raw: json.RawMessage(`{"mode":"Disabled"}`), expect: HTTPSSettings{Mode: HTTPSModeDisabled}},
		{name: "only in uri", raw: json.RawMessage(`{"mode":"OnlyInURI"}`), expect: HTTPSSettings{Mode: HTTPSModeOnlyInURI}},
		{name: "custom certificate", raw: json.RawMessage(`{"mode":"CustomCertificate","customCertificate":{"secretName":"corp"}}`), expect: HTTPSSettings{Mode: HTTPSModeCustomCertificate, CustomCertificateSecret: "corp"}},
		{name: "missing secret", raw: json.RawMessage(`{"mode":"CustomCertificate"}`), wantErr: "secretName must be set"},
		{name: "issuer override", raw: json.RawMessage(`{"mode":"CertManager","certManager":{"clusterIssuerName":"issuer"}}`), expect: HTTPSSettings{Mode: HTTPSModeCertManager, CertManagerIssuer: "issuer"}},
		{name: "globals", globals: GlobalValues{Mode: "CustomCertificate", CustomSecret: "global"}, expect: HTTPSSettings{Mode: HTTPSModeCustomCertificate, CustomCertificateSecret: "global"}},
		{name: "globals cert manager", globals: GlobalValues{Mode: "CertManager", CertManagerIssuer: "global-issuer"}, expect: HTTPSSettings{Mode: HTTPSModeCertManager, CertManagerIssuer: "global-issuer"}},
		{
			name:    "raw overrides globals",
			raw:     json.RawMessage(`{"mode":"Disabled"}`),
			globals: GlobalValues{Mode: "CustomCertificate", CustomSecret: "global"},
			expect:  HTTPSSettings{Mode: HTTPSModeDisabled},
		},
		{name: "decode error", raw: json.RawMessage(`"oops"`), wantErr: "decode https settings"},
		{name: "unknown mode", raw: json.RawMessage(`{"mode":"unsupported"}`), wantErr: "unknown https.mode"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, mapped, err := parseHTTPS(tc.raw, tc.globals)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expect {
				t.Fatalf("unexpected HTTPS settings: %+v", got)
			}
			if tc.wantErr != "" {
				return
			}
			if mapped == nil {
				t.Fatalf("expected mapped https data")
			}
			switch tc.expect.Mode {
			case HTTPSModeCustomCertificate:
				cert, ok := mapped["customCertificate"].(map[string]any)
				if !ok {
					t.Fatalf("expected customCertificate map in sanitized data")
				}
				if cert["secretName"].(string) != tc.expect.CustomCertificateSecret {
					t.Fatalf("unexpected secret in sanitized data: %v", cert["secretName"])
				}
			case HTTPSModeCertManager:
				manager, ok := mapped["certManager"].(map[string]any)
				if !ok {
					t.Fatalf("expected certManager map in sanitized data")
				}
				if manager["clusterIssuerName"].(string) != tc.expect.CertManagerIssuer {
					t.Fatalf("unexpected issuer in sanitized data: %v", manager["clusterIssuerName"])
				}
			default:
				if len(mapped) != 1 || mapped["mode"] != string(tc.expect.Mode) {
					t.Fatalf("unexpected sanitized data for mode %s: %+v", tc.expect.Mode, mapped)
				}
			}
		})
	}
}

func TestParseApproval(t *testing.T) {
	cases := []struct {
		name         string
		raw          json.RawMessage
		expectMode   DeviceApprovalMode
		wantSelector bool
		wantErr      string
	}{
		{name: "defaults", expectMode: DeviceApprovalModeManual},
		{name: "null raw", raw: json.RawMessage("null"), expectMode: DeviceApprovalModeManual},
		{name: "automatic", raw: json.RawMessage(`{"mode":"Automatic"}`), expectMode: DeviceApprovalModeAutomatic},
		{name: "trimmed mode", raw: json.RawMessage(`{"mode":"  Selector  "}`), expectMode: DeviceApprovalModeSelector, wantSelector: false},
		{name: "selector missing body", raw: json.RawMessage(`{"mode":"Selector"}`), expectMode: DeviceApprovalModeSelector, wantSelector: false},
		{name: "selector with body", raw: json.RawMessage(`{"mode":"Selector","selector":{"matchLabels":{"gpu":"true"}}}`), expectMode: DeviceApprovalModeSelector, wantSelector: true},
		{name: "invalid", raw: json.RawMessage(`{"mode":"unknown"}`), wantErr: "unknown deviceApproval.mode"},
		{name: "decode error", raw: json.RawMessage(`"oops"`), wantErr: "decode deviceApproval"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, mapped, err := parseApproval(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Mode != tc.expectMode {
				t.Fatalf("unexpected mode: %s", got.Mode)
			}
			if tc.wantSelector && (got.Selector == nil || mapped == nil) {
				t.Fatalf("expected selector and mapped output")
			}
			if !tc.wantSelector && (got.Selector != nil || mapped != nil) {
				t.Fatalf("expected selector to be nil")
			}
		})
	}
}

func TestParseBool(t *testing.T) {
	if parseBool(nil) != nil {
		t.Fatalf("expected nil for nil input")
	}
	if parseBool(json.RawMessage(`"true"`)) != nil {
		t.Fatalf("expected nil for string")
	}
	if flag := parseBool(json.RawMessage(`true`)); flag == nil || !*flag {
		t.Fatalf("expected boolean true")
	}
}

func TestDeepCopySanitizedMap(t *testing.T) {
	src := map[string]any{
		"map":   map[string]any{"key": "value"},
		"slice": []any{"a", "b"},
		"num":   1,
	}
	dst := deepCopySanitizedMap(src)
	dst["map"].(map[string]any)["key"] = "changed"
	dst["slice"].([]any)[0] = "x"

	if src["map"].(map[string]any)["key"].(string) != "value" {
		t.Fatalf("expected map deep copy")
	}
	if src["slice"].([]any)[0].(string) != "a" {
		t.Fatalf("expected slice deep copy")
	}
	if deepCopySanitizedMap(nil) != nil {
		t.Fatalf("expected nil copy")
	}
}

func TestSelectorToMap(t *testing.T) {
	selector := metav1.LabelSelector{
		MatchLabels: map[string]string{"key": "value"},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "gpu", Operator: metav1.LabelSelectorOpIn, Values: []string{"true"}},
		},
	}
	out := selectorToMap(selector)
	if out["matchLabels"].(map[string]string)["key"] != "value" {
		t.Fatalf("unexpected match labels")
	}
	if len(out["matchExpressions"].([]map[string]any)) != 1 {
		t.Fatalf("unexpected match expressions")
	}
	if selectorToMap(metav1.LabelSelector{}) != nil {
		t.Fatalf("expected nil for empty selector")
	}
}

func TestDeepCopyValueVariants(t *testing.T) {
	originalMap := map[string]string{"k": "v"}
	copied := deepCopyValue(originalMap).(map[string]string)
	copied["k"] = "x"
	if originalMap["k"] != "v" {
		t.Fatalf("expected original map unchanged")
	}

	originalSlice := []string{"a", "b"}
	sliceCopy := deepCopyValue(originalSlice).([]string)
	sliceCopy[0] = "x"
	if originalSlice[0] != "a" {
		t.Fatalf("expected original slice unchanged")
	}

	if deepCopyValue(123).(int) != 123 {
		t.Fatalf("expected primitive unchanged")
	}
}
