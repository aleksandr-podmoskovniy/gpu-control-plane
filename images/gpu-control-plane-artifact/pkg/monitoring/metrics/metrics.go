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

var (
	metricStorage metricsstorage.Storage
	registerOnce  = new(sync.Once)
)

// Register adds metrics storage to the controller-runtime metrics registry.
func Register() {
	registerOnce.Do(func() {
		ms := metricsstorage.NewMetricStorage(metricsstorage.WithNewRegistry())

		if err := crmetrics.Registry.Register(ms.Collector()); err != nil {
			panic(fmt.Errorf("register metrics storage: %w", err))
		}

		metricStorage = ms
	})
}

func Registerer() metricsstorage.Registerer {
	Register()
	return metricStorage
}

func GroupedStorage() metricsstorage.GroupedStorage {
	Register()
	return metricStorage.Grouped()
}

func MustRegisterGauge(storage metricsstorage.Registerer, metric string, labelNames []string, help string) {
	_, err := storage.RegisterGauge(metric, labelNames, msoptions.WithHelp(help))
	if err != nil {
		panic(fmt.Errorf("register gauge %q: %w", metric, err))
	}
}

func MustRegisterCounter(storage metricsstorage.Registerer, metric string, labelNames []string, help string) {
	_, err := storage.RegisterCounter(metric, labelNames, msoptions.WithHelp(help))
	if err != nil {
		panic(fmt.Errorf("register counter %q: %w", metric, err))
	}
}
