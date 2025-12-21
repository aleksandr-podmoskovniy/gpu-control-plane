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

package webhook

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

func selectSinglePool(pod *corev1.Pod) (poolRequest, bool, error) {
	pools := collectPools(pod)
	if len(pools) == 0 {
		return poolRequest{}, false, nil
	}
	if len(pools) == 1 {
		for _, p := range pools {
			return p, true, nil
		}
	}

	names := make([]string, 0, len(pools))
	for _, p := range pools {
		names = append(names, p.keyPrefix+p.name)
	}
	sort.Strings(names)
	return poolRequest{}, false, fmt.Errorf("multiple GPU pools requested: %v", names)
}

// collectPools returns a set of pools referenced in all containers (requests/limits).
func collectPools(pod *corev1.Pod) map[string]poolRequest {
	pools := make(map[string]poolRequest)
	check := func(resources corev1.ResourceList) {
		for res := range resources {
			name := res.String()
			switch {
			case strings.HasPrefix(name, localPoolResourcePrefix):
				pool := strings.TrimPrefix(name, localPoolResourcePrefix)
				if pool != "" {
					pools[localPoolResourcePrefix+pool] = poolRequest{name: pool, keyPrefix: localPoolResourcePrefix}
				}
			case strings.HasPrefix(name, clusterPoolResourcePrefix):
				pool := strings.TrimPrefix(name, clusterPoolResourcePrefix)
				if pool != "" {
					pools[clusterPoolResourcePrefix+pool] = poolRequest{name: pool, keyPrefix: clusterPoolResourcePrefix}
				}
			}
		}
	}

	for _, c := range pod.Spec.Containers {
		check(c.Resources.Limits)
		check(c.Resources.Requests)
	}
	for _, c := range pod.Spec.InitContainers {
		check(c.Resources.Limits)
		check(c.Resources.Requests)
	}
	return pools
}

func poolLabelKey(pool poolRequest) string { return pool.keyPrefix + pool.name }
