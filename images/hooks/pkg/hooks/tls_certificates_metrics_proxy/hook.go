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
	"context"
	"fmt"
	"strings"

	tlscertificate "github.com/deckhouse/module-sdk/common-hooks/tls-certificate"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"

	"hooks/pkg/settings"
)

var _ = registry.RegisterFunc(&pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 4},
	Queue:        settings.ModuleQueue,
}, ensureMetricsValues)

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
	}),
	FullValuesPathPrefix: settings.InternalMetricsCertPath,
	CommonCAValuesPath:   settings.InternalRootCAPath,
})

func ensureMetricsValues(_ context.Context, input *pkg.HookInput) error {
	ensureMap(input, settings.InternalMetricsPath)
	ensureMap(input, settings.InternalMetricsCertPath)
	return nil
}

func ensureMap(input *pkg.HookInput, path string) {
	if path == "" {
		return
	}

	current := input.Values.Get(path)
	if current.Exists() && current.IsObject() {
		return
	}

	if idx := strings.LastIndex(path, "."); idx != -1 {
		parent := path[:idx]
		if parent != "" {
			ensureMap(input, parent)
		}
	}

	input.Values.Set(path, map[string]any{})
}
