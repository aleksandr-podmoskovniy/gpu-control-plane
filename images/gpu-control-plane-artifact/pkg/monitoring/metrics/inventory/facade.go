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

func InventoryDevicesSet(node string, count int) {
	if node == "" {
		return
	}

	groupedStorage().GaugeSet(node, InventoryDevicesTotalMetric, float64(count), map[string]string{
		"node": node,
	})
}

func InventoryDevicesDelete(node string) {
	if node == "" {
		return
	}

	groupedStorage().ExpireGroupMetricByName(node, InventoryDevicesTotalMetric)
}

func InventoryConditionSet(node, condition string, value bool) {
	if node == "" || condition == "" {
		return
	}

	group := node + "|" + condition
	groupedStorage().GaugeSet(group, InventoryConditionMetric, boolToFloat(value), map[string]string{
		"node":      node,
		"condition": condition,
	})
}

func InventoryConditionDelete(node, condition string) {
	if node == "" || condition == "" {
		return
	}

	group := node + "|" + condition
	groupedStorage().ExpireGroupMetricByName(group, InventoryConditionMetric)
}

func InventoryDeviceStateSet(node, state string, count int) {
	if node == "" || state == "" {
		return
	}

	group := node + "|" + state
	groupedStorage().GaugeSet(group, InventoryDeviceStateMetric, float64(count), map[string]string{
		"node":  node,
		"state": state,
	})
}

func InventoryDeviceStateDelete(node, state string) {
	if node == "" || state == "" {
		return
	}

	group := node + "|" + state
	groupedStorage().ExpireGroupMetricByName(group, InventoryDeviceStateMetric)
}

func InventoryHandlerErrorInc(handler string) {
	if handler == "" {
		return
	}

	groupedStorage().CounterAdd(handler, InventoryHandlerErrorsTotal, 1, map[string]string{
		"handler": handler,
	})
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
