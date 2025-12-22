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
	"strings"
	"testing"
)

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
