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

package inventory

import (
	"sync"

	metricsstorage "github.com/deckhouse/deckhouse/pkg/metrics-storage"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics"
)

var registerOnce = new(sync.Once)

func Register() {
	registerOnce.Do(func() {
		storage := metrics.Registerer()
		metrics.MustRegisterGauge(storage, InventoryDevicesTotalMetric, []string{"node"}, "Number of GPU devices discovered on a node.")
		metrics.MustRegisterGauge(storage, InventoryConditionMetric, []string{"node", "condition"}, "Inventory condition status (0 or 1).")
		metrics.MustRegisterGauge(storage, InventoryDeviceStateMetric, []string{"node", "state"}, "Number of GPU devices on a node grouped by state.")
		metrics.MustRegisterCounter(storage, InventoryHandlerErrorsTotal, []string{"handler"}, "Number of errors returned by inventory handlers.")
	})
}

func groupedStorage() metricsstorage.GroupedStorage {
	Register()
	return metrics.GroupedStorage()
}
