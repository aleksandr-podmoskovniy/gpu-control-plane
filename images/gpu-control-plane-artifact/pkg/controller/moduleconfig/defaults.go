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

const (
	DefaultNodeLabelKey           = "gpu.deckhouse.io/enabled"
	DefaultDeviceApprovalMode     = DeviceApprovalModeManual
	DefaultSchedulingStrategy     = "Spread"
	DefaultSchedulingTopology     = "topology.kubernetes.io/zone"
	DefaultMonitoringService      = true
	DefaultInventoryResyncPeriod  = ""
	DefaultLogLevel               = "Info"
	DefaultHTTPSMode              = HTTPSModeCertManager
	DefaultHTTPSCertManagerIssuer = "letsencrypt"
)

func DefaultState() State {
	settings := Settings{
		ManagedNodes:   ManagedNodesSettings{LabelKey: DefaultNodeLabelKey, EnabledByDefault: true},
		DeviceApproval: DeviceApprovalSettings{Mode: DeviceApprovalModeManual},
		Scheduling:     SchedulingSettings{DefaultStrategy: DefaultSchedulingStrategy, TopologyKey: DefaultSchedulingTopology},
		Placement:      PlacementSettings{},
		Monitoring:     MonitoringSettings{ServiceMonitor: DefaultMonitoringService},
		LogLevel:       DefaultLogLevel,
	}
	sanitized := map[string]any{
		"managedNodes":   map[string]any{"labelKey": DefaultNodeLabelKey, "enabledByDefault": true},
		"deviceApproval": map[string]any{"mode": string(DefaultDeviceApprovalMode)},
		"scheduling":     map[string]any{"defaultStrategy": DefaultSchedulingStrategy, "topologyKey": DefaultSchedulingTopology},
		"placement":      map[string]any{"customTolerationKeys": []string{}},
		"monitoring":     map[string]any{"serviceMonitor": DefaultMonitoringService},
		"logLevel":       DefaultLogLevel,
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
