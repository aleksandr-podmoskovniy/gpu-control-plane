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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestRequestedResourcesPrefersLimitsAndUsesInitMax(t *testing.T) {
	pool := poolRequest{name: "pool-a", keyPrefix: localPoolResourcePrefix}
	key := corev1.ResourceName(localPoolResourcePrefix + "pool-a")

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "c1",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{key: *resource.NewQuantity(2, resource.DecimalSI)},
					},
				},
				{
					Name: "c2",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{key: *resource.NewQuantity(1, resource.DecimalSI)},
					},
				},
			},
			InitContainers: []corev1.Container{
				{
					Name: "i1",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{key: *resource.NewQuantity(1, resource.DecimalSI)},
					},
				},
			},
		},
	}

	if got := requestedResources(pod, pool); got != 3 {
		t.Fatalf("expected sumContainers=3 to win, got %d", got)
	}

	pod.Spec.InitContainers[0].Resources.Limits[key] = *resource.NewQuantity(5, resource.DecimalSI)
	if got := requestedResources(pod, pool); got != 5 {
		t.Fatalf("expected maxInit=5 to win, got %d", got)
	}
}

func TestCollectPoolsIncludesInitContainers(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{
				Name: "init",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceName(clusterPoolResourcePrefix + "pool-a"): *resource.NewQuantity(1, resource.DecimalSI),
					},
				},
			}},
		},
	}

	pools := collectPools(pod)
	if _, ok := pools[clusterPoolResourcePrefix+"pool-a"]; !ok {
		t.Fatalf("expected pool from init containers, got %+v", pools)
	}
}

func TestRequestedResourcesReturnsZeroWhenResourceAbsent(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}}}
	if got := requestedResources(pod, poolRequest{name: "pool-a", keyPrefix: localPoolResourcePrefix}); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}
