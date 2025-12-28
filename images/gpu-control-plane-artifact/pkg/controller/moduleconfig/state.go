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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type Input struct {
	Enabled  *bool
	Settings map[string]any
	Global   GlobalValues
}

type GlobalValues struct {
	Mode              string
	CertManagerIssuer string
	CustomSecret      string
}

type State struct {
	Enabled          bool
	Settings         Settings
	Inventory        InventorySettings
	HighAvailability *bool
	HTTPS            HTTPSSettings
	Sanitized        map[string]any
}

type Settings struct {
	ManagedNodes   ManagedNodesSettings
	DeviceApproval DeviceApprovalSettings
	Scheduling     SchedulingSettings
	Placement      PlacementSettings
	Monitoring     MonitoringSettings
	LogLevel       string
}

type ManagedNodesSettings struct {
	LabelKey         string
	EnabledByDefault bool
}

type MonitoringSettings struct {
	ServiceMonitor bool
}

type DeviceApprovalMode string

const (
	DeviceApprovalModeManual    DeviceApprovalMode = "Manual"
	DeviceApprovalModeAutomatic DeviceApprovalMode = "Automatic"
	DeviceApprovalModeSelector  DeviceApprovalMode = "Selector"
)

type DeviceApprovalSettings struct {
	Mode     DeviceApprovalMode
	Selector *metav1.LabelSelector
}

type SchedulingSettings struct {
	DefaultStrategy string
	TopologyKey     string
}

type PlacementSettings struct {
	CustomTolerationKeys []string
}

type InventorySettings struct {
	ResyncPeriod string
}

type HTTPSMode string

const (
	HTTPSModeDisabled          HTTPSMode = "Disabled"
	HTTPSModeCertManager       HTTPSMode = "CertManager"
	HTTPSModeCustomCertificate HTTPSMode = "CustomCertificate"
	HTTPSModeOnlyInURI         HTTPSMode = "OnlyInURI"
)

type HTTPSSettings struct {
	Mode                    HTTPSMode
	CertManagerIssuer       string
	CustomCertificateSecret string
}
