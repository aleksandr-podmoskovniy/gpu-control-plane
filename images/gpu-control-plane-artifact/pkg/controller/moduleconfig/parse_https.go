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
)

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
