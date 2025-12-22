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

import corev1 "k8s.io/api/core/v1"

func requestedResources(pod *corev1.Pod, pool poolRequest) int64 {
	name := corev1.ResourceName(pool.keyPrefix + pool.name)
	value := func(req corev1.ResourceRequirements) int64 {
		if q, ok := req.Limits[name]; ok {
			return q.Value()
		}
		if q, ok := req.Requests[name]; ok {
			return q.Value()
		}
		return 0
	}

	var sumContainers int64
	for _, c := range pod.Spec.Containers {
		sumContainers += value(c.Resources)
	}

	var maxInit int64
	for _, c := range pod.Spec.InitContainers {
		if v := value(c.Resources); v > maxInit {
			maxInit = v
		}
	}

	if sumContainers > maxInit {
		return sumContainers
	}
	return maxInit
}
