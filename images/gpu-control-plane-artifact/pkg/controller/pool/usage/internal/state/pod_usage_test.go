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

package state

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodCountsTowardsUsage(t *testing.T) {
	resourceName := corev1.ResourceName("gpu.deckhouse.io/pool-a")

	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{name: "nil", pod: nil, want: false},
		{
			name: "unscheduled",
			pod: &corev1.Pod{
				Spec:   corev1.PodSpec{Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{resourceName: resource.MustParse("1")}}}}},
				Status: corev1.PodStatus{Phase: corev1.PodPending},
			},
			want: false,
		},
		{
			name: "succeeded",
			pod: &corev1.Pod{
				Spec:   corev1.PodSpec{NodeName: "node1"},
				Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
			},
			want: false,
		},
		{
			name: "failed",
			pod: &corev1.Pod{
				Spec:   corev1.PodSpec{NodeName: "node1"},
				Status: corev1.PodStatus{Phase: corev1.PodFailed},
			},
			want: false,
		},
		{
			name: "running",
			pod: &corev1.Pod{
				Spec:   corev1.PodSpec{NodeName: "node1"},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
			want: true,
		},
		{
			name: "node-name-whitespace",
			pod: &corev1.Pod{
				Spec:   corev1.PodSpec{NodeName: " \n\t "},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PodCountsTowardsUsage(tc.pod)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestRequestedResources(t *testing.T) {
	resourceName := corev1.ResourceName("gpu.deckhouse.io/pool-a")

	tests := []struct {
		name string
		pod  *corev1.Pod
		want int64
	}{
		{name: "nil", pod: nil, want: 0},
		{
			name: "no-resources",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c"}},
				},
			},
			want: 0,
		},
		{
			name: "requests-when-no-limits",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "c",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{resourceName: resource.MustParse("2")},
						},
					}},
				},
			},
			want: 2,
		},
		{
			name: "limits-prefer-over-requests",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "c",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{resourceName: resource.MustParse("3")},
							Requests: corev1.ResourceList{resourceName: resource.MustParse("2")},
						},
					}},
				},
			},
			want: 3,
		},
		{
			name: "init-takes-max",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name: "init",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{resourceName: resource.MustParse("4")},
						},
					}},
					Containers: []corev1.Container{{
						Name: "c",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{resourceName: resource.MustParse("1")},
						},
					}},
				},
			},
			want: 4,
		},
		{
			name: "sum-containers-wins",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name: "init",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{resourceName: resource.MustParse("1")},
						},
					}},
					Containers: []corev1.Container{
						{
							Name: "c1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{resourceName: resource.MustParse("2")},
							},
						},
						{
							Name: "c2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{resourceName: resource.MustParse("2")},
							},
						},
					},
				},
			},
			want: 4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RequestedResources(tc.pod, resourceName)
			if got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestClampInt64ToInt32(t *testing.T) {
	maxInt32 := int64(^uint32(0) >> 1)

	tests := []struct {
		name string
		in   int64
		want int32
	}{
		{name: "negative", in: -1, want: 0},
		{name: "zero", in: 0, want: 0},
		{name: "small", in: 123, want: 123},
		{name: "max", in: maxInt32, want: int32(maxInt32)},
		{name: "overflow", in: maxInt32 + 1, want: int32(maxInt32)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClampInt64ToInt32(tc.in)
			if got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}
