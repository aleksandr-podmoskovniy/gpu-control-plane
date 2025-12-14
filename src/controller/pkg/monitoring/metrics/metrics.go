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

package metrics

import (
	"fmt"
	"sync"

	metricsstorage "github.com/deckhouse/deckhouse/pkg/metrics-storage"
	msoptions "github.com/deckhouse/deckhouse/pkg/metrics-storage/options"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	InventoryDevicesTotalMetric = "gpu_inventory_devices_total"
	InventoryConditionMetric    = "gpu_inventory_condition"
	InventoryDeviceStateMetric  = "gpu_inventory_devices_state"
	InventoryHandlerErrorsTotal = "gpu_inventory_handler_errors_total"

	BootstrapNodePhaseMetric    = "gpu_bootstrap_node_phase"
	BootstrapConditionMetric    = "gpu_bootstrap_condition"
	BootstrapHandlerErrorsTotal = "gpu_bootstrap_handler_errors_total"
)

var (
	metricStorage metricsstorage.Storage
	registerOnce  = new(sync.Once)
)

// Register adds all controller metrics to the controller-runtime metrics registry.
func Register() {
	registerOnce.Do(func() {
		ms := metricsstorage.NewMetricStorage(metricsstorage.WithNewRegistry())

		if err := crmetrics.Registry.Register(ms.Collector()); err != nil {
			panic(fmt.Errorf("register metrics storage: %w", err))
		}

		mustRegisterGauge(ms, InventoryDevicesTotalMetric, []string{"node"}, "Number of GPU devices discovered on a node.")
		mustRegisterGauge(ms, InventoryConditionMetric, []string{"node", "condition"}, "Inventory condition status (0 or 1).")
		mustRegisterGauge(ms, InventoryDeviceStateMetric, []string{"node", "state"}, "Number of GPU devices on a node grouped by state.")
		mustRegisterCounter(ms, InventoryHandlerErrorsTotal, []string{"handler"}, "Number of errors returned by inventory handlers.")

		mustRegisterGauge(ms, BootstrapNodePhaseMetric, []string{"node", "phase"}, "Current bootstrap phase per node.")
		mustRegisterGauge(ms, BootstrapConditionMetric, []string{"node", "condition"}, "Bootstrap conditions that are true for a node.")
		mustRegisterCounter(ms, BootstrapHandlerErrorsTotal, []string{"handler"}, "Number of bootstrap handler failures.")

		metricStorage = ms
	})
}

func mustRegisterGauge(storage metricsstorage.Registerer, metric string, labelNames []string, help string) {
	_, err := storage.RegisterGauge(metric, labelNames, msoptions.WithHelp(help))
	if err != nil {
		panic(fmt.Errorf("register gauge %q: %w", metric, err))
	}
}

func mustRegisterCounter(storage metricsstorage.Registerer, metric string, labelNames []string, help string) {
	_, err := storage.RegisterCounter(metric, labelNames, msoptions.WithHelp(help))
	if err != nil {
		panic(fmt.Errorf("register counter %q: %w", metric, err))
	}
}

func groupedStorage() metricsstorage.GroupedStorage {
	Register()
	return metricStorage.Grouped()
}
