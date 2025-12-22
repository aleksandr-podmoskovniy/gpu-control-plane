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
	"strings"
	"testing"
)

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
