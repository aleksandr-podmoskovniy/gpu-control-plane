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

package config

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
)

func TestModuleSettingsToState(t *testing.T) {
	settings := ModuleSettings{
		ManagedNodes: ManagedNodesSettings{
			LabelKey:         "gpu.deckhouse.io/custom",
			EnabledByDefault: false,
		},
		DeviceApproval: DeviceApprovalSettings{
			Mode: DeviceApprovalModeSelector,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"gpu.deckhouse.io/product": "A100"},
			},
		},
		Scheduling: SchedulingSettings{
			DefaultStrategy: "BinPack",
		},
		Monitoring: MonitoringSettings{
			ServiceMonitor: false,
		},
		Inventory: InventorySettings{
			ResyncPeriod: "5m",
		},
		HTTPS: HTTPSSettings{
			Mode:                    HTTPSModeCustomCertificate,
			CertManagerIssuer:       "ignored",
			CustomCertificateSecret: "my-secret",
		},
		HighAvailability: boolPtr(true),
	}

	state, err := ModuleSettingsToState(settings)
	if err != nil {
		t.Fatalf("ModuleSettingsToState returned error: %v", err)
	}

	if state.Settings.ManagedNodes.LabelKey != "gpu.deckhouse.io/custom" {
		t.Fatalf("unexpected managed label key: %s", state.Settings.ManagedNodes.LabelKey)
	}
	if state.Settings.ManagedNodes.EnabledByDefault {
		t.Fatalf("expected enabled by default to be false")
	}
	if state.Settings.DeviceApproval.Mode != "Selector" {
		t.Fatalf("unexpected device approval mode: %s", state.Settings.DeviceApproval.Mode)
	}
	if state.Settings.DeviceApproval.Selector == nil {
		t.Fatalf("selector should be populated")
	}
	if state.Settings.Scheduling.DefaultStrategy != "BinPack" {
		t.Fatalf("unexpected scheduling strategy: %s", state.Settings.Scheduling.DefaultStrategy)
	}
	if state.Settings.Scheduling.TopologyKey != "" {
		t.Fatalf("topology key should be empty when not provided")
	}
	if state.Settings.Monitoring.ServiceMonitor {
		t.Fatalf("expected monitoring service monitor false when disabled explicitly")
	}
	if state.Inventory.ResyncPeriod != "5m" {
		t.Fatalf("unexpected inventory resync period: %s", state.Inventory.ResyncPeriod)
	}
	if state.HTTPS.Mode != moduleconfig.HTTPSModeCustomCertificate || state.HTTPS.CustomCertificateSecret != "my-secret" {
		t.Fatalf("unexpected https settings: %+v", state.HTTPS)
	}
	if state.HighAvailability == nil || !*state.HighAvailability {
		t.Fatalf("expected highAvailability true, got %+v", state.HighAvailability)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
