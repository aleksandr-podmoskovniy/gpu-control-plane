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

package tls_certificates_controller

import (
	"testing"

	pkg "github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"

	"hooks/pkg/settings"
)

func newHookInput(t *testing.T, values map[string]any) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	t.Helper()

	patchable, err := patchablevalues.NewPatchableValues(values)
	if err != nil {
		t.Fatalf("create patchable values: %v", err)
	}

	return &pkg.HookInput{Values: patchable}, patchable
}

func TestControllerTLSHookConf(t *testing.T) {
	conf := controllerTLSHookConf()

	if conf.CN != settings.ControllerCertCN {
		t.Fatalf("unexpected CN: %s", conf.CN)
	}
	if conf.Namespace != settings.ModuleNamespace {
		t.Fatalf("unexpected namespace: %s", conf.Namespace)
	}
	if conf.TLSSecretName != settings.ControllerTLSSecretName {
		t.Fatalf("unexpected TLS secret name: %s", conf.TLSSecretName)
	}
	if conf.FullValuesPathPrefix != settings.InternalControllerCertPath {
		t.Fatalf("unexpected controller cert path: %s", conf.FullValuesPathPrefix)
	}
	if conf.CommonCAValuesPath != settings.InternalRootCAPath {
		t.Fatalf("unexpected common CA path: %s", conf.CommonCAValuesPath)
	}

	input, _ := newHookInput(t, map[string]any{
		"global": map[string]any{
			"discovery": map[string]any{
				"clusterDomain": "",
			},
			"modules": map[string]any{
				"publicDomainTemplate": "",
			},
		},
	})

	expectedSANs := map[string]struct{}{
		"localhost":               {},
		"127.0.0.1":               {},
		settings.ControllerCertCN: {},
		settings.ControllerCertCN + "." + settings.ModuleNamespace:          {},
		settings.ControllerCertCN + "." + settings.ModuleNamespace + ".svc": {},
	}

	actualSANs := conf.SANs(input)
	if len(actualSANs) != len(expectedSANs) {
		t.Fatalf("unexpected SANs length: %d", len(actualSANs))
	}

	for _, san := range actualSANs {
		if _, ok := expectedSANs[san]; !ok {
			t.Fatalf("unexpected SAN %q in configuration", san)
		}
		delete(expectedSANs, san)
	}

	if len(expectedSANs) != 0 {
		t.Fatalf("missing SAN entries: %#v", expectedSANs)
	}
}
