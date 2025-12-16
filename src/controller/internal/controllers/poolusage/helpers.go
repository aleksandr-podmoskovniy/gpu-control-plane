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

package poolusage

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

func podCountsTowardsUsage(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	if strings.TrimSpace(pod.Spec.NodeName) == "" {
		return false
	}
	switch pod.Status.Phase {
	case corev1.PodSucceeded, corev1.PodFailed:
		return false
	default:
		return true
	}
}

func requestedResources(pod *corev1.Pod, resource corev1.ResourceName) int64 {
	if pod == nil {
		return 0
	}
	value := func(req corev1.ResourceRequirements) int64 {
		if q, ok := req.Limits[resource]; ok {
			return q.Value()
		}
		if q, ok := req.Requests[resource]; ok {
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

func clampInt64ToInt32(value int64) int32 {
	if value < 0 {
		return 0
	}
	if value > int64(^uint32(0)>>1) {
		return int32(^uint32(0) >> 1)
	}
	return int32(value)
}
