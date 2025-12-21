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

package metrics_test

import (
	"strings"
	"testing"

	promdto "github.com/prometheus/client_model/go"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	bootmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics/bootstrap"
	invmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics/inventory"
)

func TestInventoryMetricsFacadeSetAndDelete(t *testing.T) {
	node := "node-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	cond := "cond-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	state := "state-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))

	invmetrics.InventoryDevicesSet("", 10)
	if _, ok := findMetric(t, invmetrics.InventoryDevicesTotalMetric, map[string]string{"node": ""}); ok {
		t.Fatalf("expected empty node to be ignored")
	}

	invmetrics.InventoryDevicesSet(node, 2)
	if v, ok := gaugeValue(t, invmetrics.InventoryDevicesTotalMetric, map[string]string{"node": node}); !ok || v != 2 {
		t.Fatalf("expected inventory devices gauge=2, got %f (present=%t)", v, ok)
	}
	invmetrics.InventoryDevicesDelete(node)
	if _, ok := findMetric(t, invmetrics.InventoryDevicesTotalMetric, map[string]string{"node": node}); ok {
		t.Fatalf("expected inventory devices gauge cleared")
	}

	invmetrics.InventoryConditionSet(node, cond, true)
	if v, ok := gaugeValue(t, invmetrics.InventoryConditionMetric, map[string]string{"node": node, "condition": cond}); !ok || v != 1 {
		t.Fatalf("expected inventory condition gauge=1, got %f (present=%t)", v, ok)
	}
	invmetrics.InventoryConditionSet(node, cond, false)
	if v, ok := gaugeValue(t, invmetrics.InventoryConditionMetric, map[string]string{"node": node, "condition": cond}); !ok || v != 0 {
		t.Fatalf("expected inventory condition gauge=0, got %f (present=%t)", v, ok)
	}
	invmetrics.InventoryConditionDelete(node, cond)
	if _, ok := findMetric(t, invmetrics.InventoryConditionMetric, map[string]string{"node": node, "condition": cond}); ok {
		t.Fatalf("expected inventory condition gauge cleared")
	}

	invmetrics.InventoryDeviceStateSet(node, state, 3)
	if v, ok := gaugeValue(t, invmetrics.InventoryDeviceStateMetric, map[string]string{"node": node, "state": state}); !ok || v != 3 {
		t.Fatalf("expected inventory device state gauge=3, got %f (present=%t)", v, ok)
	}
	invmetrics.InventoryDeviceStateDelete(node, state)
	if _, ok := findMetric(t, invmetrics.InventoryDeviceStateMetric, map[string]string{"node": node, "state": state}); ok {
		t.Fatalf("expected inventory device state gauge cleared")
	}
}

func TestHandlerErrorCounters(t *testing.T) {
	handlerInventory := "handler-inv-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	handlerBootstrap := "handler-boot-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))

	beforeInv := counterValueOrZero(t, invmetrics.InventoryHandlerErrorsTotal, map[string]string{"handler": handlerInventory})
	beforeBoot := counterValueOrZero(t, bootmetrics.BootstrapHandlerErrorsTotal, map[string]string{"handler": handlerBootstrap})

	invmetrics.InventoryHandlerErrorInc(handlerInventory)
	invmetrics.InventoryHandlerErrorInc(handlerInventory)
	gotInv := counterValueOrZero(t, invmetrics.InventoryHandlerErrorsTotal, map[string]string{"handler": handlerInventory})
	if gotInv-beforeInv != 2 {
		t.Fatalf("expected inventory handler errors counter to increase by 2, got delta=%f", gotInv-beforeInv)
	}

	bootmetrics.BootstrapHandlerErrorInc(handlerBootstrap)
	gotBoot := counterValueOrZero(t, bootmetrics.BootstrapHandlerErrorsTotal, map[string]string{"handler": handlerBootstrap})
	if gotBoot-beforeBoot != 1 {
		t.Fatalf("expected bootstrap handler errors counter to increase by 1, got delta=%f", gotBoot-beforeBoot)
	}
}

func TestBootstrapMetricsFacadeSetAndDelete(t *testing.T) {
	node := "node-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	phase := "phase-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	cond := "cond-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))

	bootmetrics.BootstrapPhaseSet(node, phase)
	if v, ok := gaugeValue(t, bootmetrics.BootstrapNodePhaseMetric, map[string]string{"node": node, "phase": phase}); !ok || v != 1 {
		t.Fatalf("expected bootstrap phase gauge=1, got %f (present=%t)", v, ok)
	}
	bootmetrics.BootstrapPhaseDelete(node, phase)
	if _, ok := findMetric(t, bootmetrics.BootstrapNodePhaseMetric, map[string]string{"node": node, "phase": phase}); ok {
		t.Fatalf("expected bootstrap phase gauge cleared")
	}

	bootmetrics.BootstrapConditionSet(node, cond, true)
	if v, ok := gaugeValue(t, bootmetrics.BootstrapConditionMetric, map[string]string{"node": node, "condition": cond}); !ok || v != 1 {
		t.Fatalf("expected bootstrap condition gauge=1, got %f (present=%t)", v, ok)
	}
	bootmetrics.BootstrapConditionSet(node, cond, false)
	if _, ok := findMetric(t, bootmetrics.BootstrapConditionMetric, map[string]string{"node": node, "condition": cond}); ok {
		t.Fatalf("expected bootstrap condition gauge cleared")
	}
}

func TestFacadeFunctionsIgnoreEmptyInputs(t *testing.T) {
	invmetrics.InventoryDevicesDelete("")
	invmetrics.InventoryConditionSet("", "cond", true)
	invmetrics.InventoryConditionSet("node", "", true)
	invmetrics.InventoryConditionDelete("", "cond")
	invmetrics.InventoryConditionDelete("node", "")
	invmetrics.InventoryDeviceStateSet("", "state", 1)
	invmetrics.InventoryDeviceStateSet("node", "", 1)
	invmetrics.InventoryDeviceStateDelete("", "state")
	invmetrics.InventoryDeviceStateDelete("node", "")
	invmetrics.InventoryHandlerErrorInc("")

	bootmetrics.BootstrapPhaseSet("", "phase")
	bootmetrics.BootstrapPhaseSet("node", "")
	bootmetrics.BootstrapPhaseDelete("", "phase")
	bootmetrics.BootstrapPhaseDelete("node", "")
	bootmetrics.BootstrapConditionSet("", "cond", true)
	bootmetrics.BootstrapConditionSet("node", "", true)
	bootmetrics.BootstrapConditionDelete("", "cond")
	bootmetrics.BootstrapConditionDelete("node", "")
	bootmetrics.BootstrapHandlerErrorInc("")
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
