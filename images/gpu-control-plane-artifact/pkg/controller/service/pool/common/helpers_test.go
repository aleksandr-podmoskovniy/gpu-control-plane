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

package common

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestPoolResourcePrefixAndAssignmentKeyByPoolKind(t *testing.T) {
	nsPool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}
	if got := PoolResourcePrefixFor(nsPool); got != NamespacedPoolResourcePrefix {
		t.Fatalf("unexpected namespaced prefix: %s", got)
	}
	if got := AssignmentAnnotationKey(nsPool); got != NamespacedAssignmentAnnotation {
		t.Fatalf("unexpected namespaced assignment key: %s", got)
	}

	clusterPool := &v1alpha1.GPUPool{TypeMeta: metav1.TypeMeta{Kind: "ClusterGPUPool"}, ObjectMeta: metav1.ObjectMeta{Name: "p"}}
	if got := PoolResourcePrefixFor(clusterPool); got != ClusterPoolResourcePrefix {
		t.Fatalf("unexpected cluster prefix: %s", got)
	}
	if got := AssignmentAnnotationKey(clusterPool); got != ClusterAssignmentAnnotation {
		t.Fatalf("unexpected cluster assignment key: %s", got)
	}
}

func TestIsDeviceIgnored(t *testing.T) {
	if IsDeviceIgnored(nil) {
		t.Fatalf("expected nil device to be not ignored")
	}

	dev := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{DeviceIgnoreKey: "TrUe"}}}
	if !IsDeviceIgnored(dev) {
		t.Fatalf("expected ignore label to be case-insensitive true")
	}

	dev.Labels[DeviceIgnoreKey] = "false"
	if IsDeviceIgnored(dev) {
		t.Fatalf("expected ignore label false to not ignore")
	}
}

func TestDeviceNodeName(t *testing.T) {
	if DeviceNodeName(nil) != "" {
		t.Fatalf("expected empty nodeName for nil device")
	}

	dev := &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{NodeName: " node-a "}}
	if DeviceNodeName(dev) != "node-a" {
		t.Fatalf("expected nodeName from status")
	}

	dev.Status.NodeName = ""
	if DeviceNodeName(dev) != "" {
		t.Fatalf("expected empty nodeName without status")
	}
}

func TestPoolRefMatchesPool(t *testing.T) {
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns"}}
	if PoolRefMatchesPool(nil, &v1alpha1.GPUPoolReference{Name: "pool"}) {
		t.Fatalf("expected nil pool to not match")
	}
	if PoolRefMatchesPool(pool, nil) {
		t.Fatalf("expected nil ref to not match")
	}
	if PoolRefMatchesPool(pool, &v1alpha1.GPUPoolReference{Name: "other"}) {
		t.Fatalf("expected name mismatch to not match")
	}

	// Namespaced pool requires namespace in poolRef.
	if PoolRefMatchesPool(pool, &v1alpha1.GPUPoolReference{Name: "pool"}) {
		t.Fatalf("expected ref without namespace to not match")
	}
	if PoolRefMatchesPool(pool, &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "other"}) {
		t.Fatalf("expected different ref namespace to not match")
	}
	if !PoolRefMatchesPool(pool, &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "ns"}) {
		t.Fatalf("expected matching ref namespace to match")
	}

	// Cluster pool must not carry namespace in ref.
	clusterPool := &v1alpha1.GPUPool{TypeMeta: metav1.TypeMeta{Kind: "ClusterGPUPool"}, ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	if !PoolRefMatchesPool(clusterPool, &v1alpha1.GPUPoolReference{Name: "pool"}) {
		t.Fatalf("expected empty namespace ref to match cluster pool")
	}
	if PoolRefMatchesPool(clusterPool, &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "ns"}) {
		t.Fatalf("expected namespaced ref to not match cluster pool")
	}
}
