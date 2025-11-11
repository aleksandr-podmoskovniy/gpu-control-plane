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

package settings

const (
	ModuleName         = "gpu-control-plane"
	ModuleValuesName   = "gpuControlPlane"
	ModuleNamespace    = "d8-gpu-control-plane"
	DeckhouseNamespace = "d8-system"
	ModuleQueue        = "modules/" + ModuleName
	ConfigRoot         = ModuleValuesName

	ControllerAppName           = ModuleName + "-controller"
	ControllerCertCN            = "gpu-control-plane-controller"
	ControllerTLSSecretName     = "gpu-control-plane-controller-tls"
	RootCASecretName            = "gpu-control-plane-ca"
	MetricsProxyCertCN          = "gpu-control-plane-metrics"
	MetricsTLSSecretName        = "gpu-control-plane-controller-metrics-tls"
	MonitoringNamespace         = "d8-monitoring"
	BootstrapStateConfigMapName = "gpu-control-plane-bootstrap-state"

	NodeFeatureRuleName       = "deckhouse-gpu-kernel-os"
	NFDDependencyErrorMessage = "Module gpu-control-plane requires the node-feature-discovery module to be enabled"

	DefaultNodeLabelKey          = "gpu.deckhouse.io/enabled"
	DefaultAutoAssignmentMode    = "Manual"
	DefaultSchedulingStrategy    = "Spread"
	DefaultSchedulingTopology    = "topology.kubernetes.io/zone"
	DefaultServiceMonitor        = true
	DefaultInventoryResyncPeriod = "30s"
	DefaultHTTPSMode             = "CertManager"
	DefaultHTTPSClusterIssuer    = "letsencrypt"

	GFDDaemonSetName       = "nvidia-gpu-feature-discovery"
	DCGMExporterDaemonName = "nvidia-dcgm-exporter"

	BootstrapComponentValidator           = "validator"
	BootstrapComponentGPUFeatureDiscovery = "gpu-feature-discovery"
	BootstrapComponentDCGM                = "dcgm"
	BootstrapComponentDCGMExporter        = "dcgm-exporter"
	BootstrapStateNodeSuffix              = ".yaml"
)
