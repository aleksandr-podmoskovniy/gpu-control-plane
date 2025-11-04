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

	ControllerCertCN        = "gpu-control-plane-controller"
	ControllerTLSSecretName = "gpu-control-plane-controller-tls"
	RootCASecretName        = "gpu-control-plane-ca"

	NodeFeatureRuleName = "deckhouse-gpu-kernel-os"

	DefaultNodeLabelKey          = "gpu.deckhouse.io/enabled"
	DefaultAutoAssignmentMode    = "Manual"
	DefaultSchedulingStrategy    = "Spread"
	DefaultSchedulingTopology    = "topology.kubernetes.io/zone"
	DefaultInventoryResyncPeriod = "30s"
	DefaultHTTPSMode             = "CertManager"
	DefaultHTTPSClusterIssuer    = "letsencrypt"

	GFDDaemonSetName       = "nvidia-gpu-feature-discovery"
	DCGMExporterDaemonName = "nvidia-dcgm-exporter"
)
