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
