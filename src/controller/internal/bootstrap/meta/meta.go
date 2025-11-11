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

package meta

import "fmt"

const (
	// WorkloadsNamespace is the namespace where bootstrap DaemonSets live.
	WorkloadsNamespace = "d8-gpu-control-plane"
	// MonitoringNamespace is where shared monitoring resources live.
	MonitoringNamespace = "d8-monitoring"
	// StateConfigMapName stores per-node bootstrap flags.
	StateConfigMapName = "gpu-control-plane-bootstrap-state"
	// ControllerDeploymentName is the name of the controller Deployment used for ownership.
	ControllerDeploymentName = "gpu-control-plane-controller"
	// ControllerServiceMonitorName is the name of the ServiceMonitor published for controller metrics.
	ControllerServiceMonitorName = "gpu-control-plane-controller"

	componentPrefix = "gpu-control-plane"
)

// Component identifies a bootstrap workload.
type Component string

const (
	ComponentGPUFeatureDiscovery Component = "gpu-feature-discovery"
	ComponentValidator           Component = "validator"
	ComponentDCGM                Component = "dcgm"
	ComponentDCGMExporter        Component = "dcgm-exporter"
)

var managedComponents = []Component{
	ComponentGPUFeatureDiscovery,
	ComponentValidator,
	ComponentDCGM,
	ComponentDCGMExporter,
}

// AppName returns the value stored in the pod label `app` for the component.
func AppName(component Component) string {
	return fmt.Sprintf("%s-%s", componentPrefix, component)
}

// ComponentAppNames returns the `app` label values of all managed bootstrap workloads.
func ComponentAppNames() []string {
	names := make([]string, len(managedComponents))
	for i, component := range managedComponents {
		names[i] = AppName(component)
	}
	return names
}
