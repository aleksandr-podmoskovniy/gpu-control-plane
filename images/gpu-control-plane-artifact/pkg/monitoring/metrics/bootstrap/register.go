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

package bootstrap

import (
	"sync"

	metricsstorage "github.com/deckhouse/deckhouse/pkg/metrics-storage"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics"
)

var registerOnce = new(sync.Once)

func Register() {
	registerOnce.Do(func() {
		storage := metrics.Registerer()
		metrics.MustRegisterGauge(storage, BootstrapNodePhaseMetric, []string{"node", "phase"}, "Current bootstrap phase per node.")
		metrics.MustRegisterGauge(storage, BootstrapConditionMetric, []string{"node", "condition"}, "Bootstrap conditions that are true for a node.")
		metrics.MustRegisterCounter(storage, BootstrapHandlerErrorsTotal, []string{"handler"}, "Number of bootstrap handler failures.")
	})
}

func groupedStorage() metricsstorage.GroupedStorage {
	Register()
	return metrics.GroupedStorage()
}
