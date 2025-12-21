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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/podlabels"
)

func TestEnsurePoolUsageLabelsBranches(t *testing.T) {
	t.Run("sets-labels", func(t *testing.T) {
		pod := &corev1.Pod{}
		if err := ensurePoolUsageLabels(pod, localPoolReq("pool-a")); err != nil {
			t.Fatalf("ensurePoolUsageLabels: %v", err)
		}
		if pod.Labels[podlabels.PoolNameKey] != "pool-a" {
			t.Fatalf("expected pool name label to be set, got %q", pod.Labels[podlabels.PoolNameKey])
		}
		if pod.Labels[podlabels.PoolScopeKey] != podlabels.PoolScopeNamespaced {
			t.Fatalf("expected namespaced scope label, got %q", pod.Labels[podlabels.PoolScopeKey])
		}
	})

	t.Run("cluster-scope", func(t *testing.T) {
		pod := &corev1.Pod{}
		if err := ensurePoolUsageLabels(pod, clusterPoolReq("shared")); err != nil {
			t.Fatalf("ensurePoolUsageLabels: %v", err)
		}
		if pod.Labels[podlabels.PoolScopeKey] != podlabels.PoolScopeCluster {
			t.Fatalf("expected cluster scope label, got %q", pod.Labels[podlabels.PoolScopeKey])
		}
	})

	t.Run("pool-name-conflict", func(t *testing.T) {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{podlabels.PoolNameKey: "other"},
		}}
		if err := ensurePoolUsageLabels(pod, localPoolReq("pool-a")); err == nil {
			t.Fatalf("expected pool name label conflict error")
		}
	})

	t.Run("pool-scope-conflict", func(t *testing.T) {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				podlabels.PoolNameKey:  "pool-a",
				podlabels.PoolScopeKey: podlabels.PoolScopeCluster,
			},
		}}
		if err := ensurePoolUsageLabels(pod, localPoolReq("pool-a")); err == nil {
			t.Fatalf("expected pool scope label conflict error")
		}
	})
}
