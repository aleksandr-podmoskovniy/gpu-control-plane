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

package watcher

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

func TestMapPodToNamespacedPool(t *testing.T) {
	if got := MapPodToNamespacedPool(context.Background(), nil); got != nil {
		t.Fatalf("expected nil for nil pod, got %v", got)
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}
	if got := MapPodToNamespacedPool(context.Background(), pod); got != nil {
		t.Fatalf("expected nil for nil labels, got %v", got)
	}

	pod.Labels = map[string]string{
		poolcommon.PoolNameKey:  "pool-a",
		poolcommon.PoolScopeKey: poolcommon.PoolScopeCluster,
	}
	if got := MapPodToNamespacedPool(context.Background(), pod); got != nil {
		t.Fatalf("expected nil for wrong scope, got %v", got)
	}

	pod.Labels[poolcommon.PoolScopeKey] = poolcommon.PoolScopeNamespaced
	pod.Labels[poolcommon.PoolNameKey] = "  "
	if got := MapPodToNamespacedPool(context.Background(), pod); got != nil {
		t.Fatalf("expected nil for empty pool name, got %v", got)
	}

	pod.Labels[poolcommon.PoolNameKey] = "pool-a"
	got := MapPodToNamespacedPool(context.Background(), pod)
	if len(got) != 1 {
		t.Fatalf("expected 1 request, got %v", got)
	}
	if got[0].Namespace != "ns" || got[0].Name != "pool-a" {
		t.Fatalf("unexpected request: %v", got[0])
	}
}

func TestMapPodToClusterPool(t *testing.T) {
	if got := MapPodToClusterPool(context.Background(), nil); got != nil {
		t.Fatalf("expected nil for nil pod, got %v", got)
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}
	if got := MapPodToClusterPool(context.Background(), pod); got != nil {
		t.Fatalf("expected nil for nil labels, got %v", got)
	}

	pod.Labels = map[string]string{
		poolcommon.PoolNameKey:  "pool-a",
		poolcommon.PoolScopeKey: poolcommon.PoolScopeNamespaced,
	}
	if got := MapPodToClusterPool(context.Background(), pod); got != nil {
		t.Fatalf("expected nil for wrong scope, got %v", got)
	}

	pod.Labels[poolcommon.PoolScopeKey] = poolcommon.PoolScopeCluster
	pod.Labels[poolcommon.PoolNameKey] = "  "
	if got := MapPodToClusterPool(context.Background(), pod); got != nil {
		t.Fatalf("expected nil for empty pool name, got %v", got)
	}

	pod.Labels[poolcommon.PoolNameKey] = "cluster-a"
	got := MapPodToClusterPool(context.Background(), pod)
	if len(got) != 1 {
		t.Fatalf("expected 1 request, got %v", got)
	}
	if got[0].Namespace != "" || got[0].Name != "cluster-a" {
		t.Fatalf("unexpected request: %v", got[0])
	}
}
