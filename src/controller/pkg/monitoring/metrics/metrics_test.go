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
	"strings"
	"sync"
	"testing"

	metricsstorage "github.com/deckhouse/deckhouse/pkg/metrics-storage"
	"github.com/deckhouse/deckhouse/pkg/metrics-storage/collectors"
	msoptions "github.com/deckhouse/deckhouse/pkg/metrics-storage/options"
	"github.com/prometheus/client_golang/prometheus"
	promdto "github.com/prometheus/client_model/go"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestInventoryMetricsFacadeSetAndDelete(t *testing.T) {
	node := "node-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	cond := "cond-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	state := "state-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))

	InventoryDevicesSet("", 10)
	if _, ok := findMetric(t, InventoryDevicesTotalMetric, map[string]string{"node": ""}); ok {
		t.Fatalf("expected empty node to be ignored")
	}

	InventoryDevicesSet(node, 2)
	if v, ok := gaugeValue(t, InventoryDevicesTotalMetric, map[string]string{"node": node}); !ok || v != 2 {
		t.Fatalf("expected inventory devices gauge=2, got %f (present=%t)", v, ok)
	}
	InventoryDevicesDelete(node)
	if _, ok := findMetric(t, InventoryDevicesTotalMetric, map[string]string{"node": node}); ok {
		t.Fatalf("expected inventory devices gauge cleared")
	}

	InventoryConditionSet(node, cond, true)
	if v, ok := gaugeValue(t, InventoryConditionMetric, map[string]string{"node": node, "condition": cond}); !ok || v != 1 {
		t.Fatalf("expected inventory condition gauge=1, got %f (present=%t)", v, ok)
	}
	InventoryConditionSet(node, cond, false)
	if v, ok := gaugeValue(t, InventoryConditionMetric, map[string]string{"node": node, "condition": cond}); !ok || v != 0 {
		t.Fatalf("expected inventory condition gauge=0, got %f (present=%t)", v, ok)
	}
	InventoryConditionDelete(node, cond)
	if _, ok := findMetric(t, InventoryConditionMetric, map[string]string{"node": node, "condition": cond}); ok {
		t.Fatalf("expected inventory condition gauge cleared")
	}

	InventoryDeviceStateSet(node, state, 3)
	if v, ok := gaugeValue(t, InventoryDeviceStateMetric, map[string]string{"node": node, "state": state}); !ok || v != 3 {
		t.Fatalf("expected inventory device state gauge=3, got %f (present=%t)", v, ok)
	}
	InventoryDeviceStateDelete(node, state)
	if _, ok := findMetric(t, InventoryDeviceStateMetric, map[string]string{"node": node, "state": state}); ok {
		t.Fatalf("expected inventory device state gauge cleared")
	}
}

func TestHandlerErrorCounters(t *testing.T) {
	handlerInventory := "handler-inv-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	handlerBootstrap := "handler-boot-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))

	beforeInv := counterValueOrZero(t, InventoryHandlerErrorsTotal, map[string]string{"handler": handlerInventory})
	beforeBoot := counterValueOrZero(t, BootstrapHandlerErrorsTotal, map[string]string{"handler": handlerBootstrap})

	InventoryHandlerErrorInc(handlerInventory)
	InventoryHandlerErrorInc(handlerInventory)
	gotInv := counterValueOrZero(t, InventoryHandlerErrorsTotal, map[string]string{"handler": handlerInventory})
	if gotInv-beforeInv != 2 {
		t.Fatalf("expected inventory handler errors counter to increase by 2, got delta=%f", gotInv-beforeInv)
	}

	BootstrapHandlerErrorInc(handlerBootstrap)
	gotBoot := counterValueOrZero(t, BootstrapHandlerErrorsTotal, map[string]string{"handler": handlerBootstrap})
	if gotBoot-beforeBoot != 1 {
		t.Fatalf("expected bootstrap handler errors counter to increase by 1, got delta=%f", gotBoot-beforeBoot)
	}
}

func TestBootstrapMetricsFacadeSetAndDelete(t *testing.T) {
	node := "node-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	phase := "phase-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	cond := "cond-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))

	BootstrapPhaseSet(node, phase)
	if v, ok := gaugeValue(t, BootstrapNodePhaseMetric, map[string]string{"node": node, "phase": phase}); !ok || v != 1 {
		t.Fatalf("expected bootstrap phase gauge=1, got %f (present=%t)", v, ok)
	}
	BootstrapPhaseDelete(node, phase)
	if _, ok := findMetric(t, BootstrapNodePhaseMetric, map[string]string{"node": node, "phase": phase}); ok {
		t.Fatalf("expected bootstrap phase gauge cleared")
	}

	BootstrapConditionSet(node, cond, true)
	if v, ok := gaugeValue(t, BootstrapConditionMetric, map[string]string{"node": node, "condition": cond}); !ok || v != 1 {
		t.Fatalf("expected bootstrap condition gauge=1, got %f (present=%t)", v, ok)
	}
	BootstrapConditionSet(node, cond, false)
	if _, ok := findMetric(t, BootstrapConditionMetric, map[string]string{"node": node, "condition": cond}); ok {
		t.Fatalf("expected bootstrap condition gauge cleared")
	}
}

func TestFacadeFunctionsIgnoreEmptyInputs(t *testing.T) {
	InventoryDevicesDelete("")
	InventoryConditionSet("", "cond", true)
	InventoryConditionSet("node", "", true)
	InventoryConditionDelete("", "cond")
	InventoryConditionDelete("node", "")
	InventoryDeviceStateSet("", "state", 1)
	InventoryDeviceStateSet("node", "", 1)
	InventoryDeviceStateDelete("", "state")
	InventoryDeviceStateDelete("node", "")
	InventoryHandlerErrorInc("")

	BootstrapPhaseSet("", "phase")
	BootstrapPhaseSet("node", "")
	BootstrapPhaseDelete("", "phase")
	BootstrapPhaseDelete("node", "")
	BootstrapConditionSet("", "cond", true)
	BootstrapConditionSet("node", "", true)
	BootstrapConditionDelete("", "cond")
	BootstrapConditionDelete("node", "")
	BootstrapHandlerErrorInc("")
}

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
		mustRegisterGauge(reg, "metric", nil, "help")
	})

	t.Run("counter", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatalf("expected panic")
			}
		}()
		mustRegisterCounter(reg, "metric", nil, "help")
	})
}

func labelsMatch(metric *promdto.Metric, expected map[string]string) bool {
	for name, want := range expected {
		found := false
		for _, pair := range metric.Label {
			if pair.GetName() != name {
				continue
			}
			found = true
			if pair.GetValue() != want {
				return false
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func findMetric(t *testing.T, name string, labels map[string]string) (*promdto.Metric, bool) {
	t.Helper()

	families, err := crmetrics.Registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			if labelsMatch(metric, labels) {
				return metric, true
			}
		}
		return nil, false
	}

	return nil, false
}

func counterValue(t *testing.T, name string, labels map[string]string) (float64, bool) {
	t.Helper()

	metric, ok := findMetric(t, name, labels)
	if !ok || metric.Counter == nil {
		return 0, false
	}
	return metric.Counter.GetValue(), true
}

func counterValueOrZero(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()

	v, ok := counterValue(t, name, labels)
	if !ok {
		return 0
	}
	return v
}

func gaugeValue(t *testing.T, name string, labels map[string]string) (float64, bool) {
	t.Helper()

	metric, ok := findMetric(t, name, labels)
	if !ok || metric.Gauge == nil {
		return 0, false
	}
	return metric.Gauge.GetValue(), true
}
