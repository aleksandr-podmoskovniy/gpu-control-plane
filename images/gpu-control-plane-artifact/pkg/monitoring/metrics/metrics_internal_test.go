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
	"errors"
	"sync"
	"testing"

	metricsstorage "github.com/deckhouse/deckhouse/pkg/metrics-storage"
	"github.com/deckhouse/deckhouse/pkg/metrics-storage/collectors"
	msoptions "github.com/deckhouse/deckhouse/pkg/metrics-storage/options"
	"github.com/prometheus/client_golang/prometheus"
	promdto "github.com/prometheus/client_model/go"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

type failingRegistry struct{}

func (f failingRegistry) Register(prometheus.Collector) error  { return errors.New("register failed") }
func (f failingRegistry) MustRegister(...prometheus.Collector) {}
func (f failingRegistry) Unregister(prometheus.Collector) bool { return false }
func (f failingRegistry) Gather() ([]*promdto.MetricFamily, error) {
	return nil, nil
}

func TestRegisterPanicsOnRegistryError(t *testing.T) {
	origRegistry := crmetrics.Registry
	origStorage := metricStorage
	origOnce := registerOnce

	t.Cleanup(func() {
		crmetrics.Registry = origRegistry
		metricStorage = origStorage
		registerOnce = origOnce
	})

	crmetrics.Registry = failingRegistry{}
	metricStorage = nil
	registerOnce = new(sync.Once)

	defer func() {
		if recover() == nil {
			t.Fatalf("expected Register to panic on registry error")
		}
	}()
	Register()
}

type failingRegisterer struct{}

func (f failingRegisterer) RegisterCounter(string, []string, ...msoptions.RegisterOption) (*collectors.ConstCounterCollector, error) {
	return nil, errors.New("counter failed")
}

func (f failingRegisterer) RegisterGauge(string, []string, ...msoptions.RegisterOption) (*collectors.ConstGaugeCollector, error) {
	return nil, errors.New("gauge failed")
}

func (f failingRegisterer) RegisterHistogram(string, []string, []float64, ...msoptions.RegisterOption) (*collectors.ConstHistogramCollector, error) {
	return nil, errors.New("histogram failed")
}

func TestMustRegisterHelpersPanicOnErrors(t *testing.T) {
	var reg metricsstorage.Registerer = failingRegisterer{}

	t.Run("gauge", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatalf("expected panic")
			}
		}()
		MustRegisterGauge(reg, "metric", nil, "help")
	})

	t.Run("counter", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatalf("expected panic")
			}
		}()
		MustRegisterCounter(reg, "metric", nil, "help")
	})
}
