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

package tls_certificates_metrics_proxy

import (
	"fmt"
	"strings"

	tlscertificate "github.com/deckhouse/module-sdk/common-hooks/tls-certificate"
	"github.com/deckhouse/module-sdk/pkg"

	"hooks/pkg/settings"
)

var _ = tlscertificate.RegisterInternalTLSHookEM(tlscertificate.GenSelfSignedTLSHookConf{
	CN:            settings.MetricsProxyCertCN,
	TLSSecretName: settings.MetricsTLSSecretName,
	Namespace:     settings.ModuleNamespace,
	SANs: tlscertificate.DefaultSANs([]string{
		"localhost",
		"127.0.0.1",
		fmt.Sprintf("%s-metrics", settings.ControllerAppName),
		fmt.Sprintf("%s-metrics.%s", settings.ControllerAppName, settings.ModuleNamespace),
		fmt.Sprintf("%s-metrics.%s.svc", settings.ControllerAppName, settings.ModuleNamespace),
		tlscertificate.ClusterDomainSAN(fmt.Sprintf("%s-metrics.%s.svc", settings.ControllerAppName, settings.ModuleNamespace)),
	}),
	FullValuesPathPrefix: settings.InternalMetricsCertPath,
	CommonCAValuesPath:   settings.InternalRootCAPath,
	BeforeHookCheck: func(input *pkg.HookInput) bool {
		cfg := input.Values.Get(settings.InternalModuleConfigPath)
		if !cfg.Exists() || !cfg.Get("enabled").Bool() {
			return false
		}

		metrics := input.Values.Get(settings.InternalMetricsPath)
		if !metrics.Exists() || !metrics.IsObject() {
			return false
		}

		if globalCA := input.Values.Get(settings.GlobalKubeRBACProxyCAPath); globalCA.Exists() && globalCA.IsObject() {
			caData := map[string][]byte{}
			if crt := strings.TrimSpace(globalCA.Get("cert").Str); crt != "" {
				caData["crt"] = []byte(crt)
			}
			if key := strings.TrimSpace(globalCA.Get("key").Str); key != "" {
				caData["key"] = []byte(key)
			}
			if len(caData) > 0 {
				input.Values.Set(settings.InternalRootCAPath, caData)
			}
		}

		cert := metrics.Get("cert")
		if !cert.Exists() || !cert.IsObject() {
			input.Values.Set(settings.InternalMetricsCertPath, map[string]any{})
		}

		return true
	},
})
