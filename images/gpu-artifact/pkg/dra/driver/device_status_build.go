/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

import (
	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildDeviceStatuses(claim *resourceapi.ResourceClaim, driverName string) []resourceapi.AllocatedDeviceStatus {
	if claim == nil || claim.Status.Allocation == nil {
		return nil
	}
	results := claim.Status.Allocation.Devices.Results
	if len(results) == 0 {
		return nil
	}

	out := make([]resourceapi.AllocatedDeviceStatus, 0, len(results))
	now := metav1.Now()

	for _, result := range results {
		if result.Driver != driverName {
			continue
		}
		status := resourceapi.AllocatedDeviceStatus{
			Driver:     result.Driver,
			Pool:       result.Pool,
			Device:     result.Device,
			Conditions: buildDeviceConditions(result, now),
		}
		if result.ShareID != nil {
			share := string(*result.ShareID)
			status.ShareID = &share
		}
		out = append(out, status)
	}
	return out
}

func buildDeviceConditions(result resourceapi.DeviceRequestAllocationResult, now metav1.Time) []metav1.Condition {
	var out []metav1.Condition
	seen := map[string]struct{}{}

	appendCondition := func(condType string) {
		if condType == "" {
			return
		}
		if _, ok := seen[condType]; ok {
			return
		}
		out = append(out, metav1.Condition{
			Type:               condType,
			Status:             metav1.ConditionTrue,
			Reason:             deviceStatusReasonPrepared,
			Message:            deviceStatusMessagePrepared,
			LastTransitionTime: now,
		})
		seen[condType] = struct{}{}
	}

	appendCondition(deviceStatusConditionReady)
	for _, cond := range result.BindingConditions {
		appendCondition(cond)
	}

	return out
}

func mergeDeviceStatuses(existing, desired []resourceapi.AllocatedDeviceStatus, driverName string) []resourceapi.AllocatedDeviceStatus {
	if len(existing) == 0 && len(desired) == 0 {
		return nil
	}
	out := make([]resourceapi.AllocatedDeviceStatus, 0, len(existing)+len(desired))
	for _, status := range existing {
		if status.Driver == driverName {
			continue
		}
		out = append(out, status)
	}
	out = append(out, desired...)
	return out
}
