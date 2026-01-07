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

package prepare

import resourcev1 "k8s.io/api/resource/v1"

func filterResults(results []resourcev1.DeviceRequestAllocationResult, driverName string) []resourcev1.DeviceRequestAllocationResult {
	if driverName == "" {
		return results
	}
	filtered := make([]resourcev1.DeviceRequestAllocationResult, 0, len(results))
	for _, res := range results {
		if res.Driver != driverName {
			continue
		}
		filtered = append(filtered, res)
	}
	return filtered
}
