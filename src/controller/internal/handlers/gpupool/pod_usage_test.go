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

package gpupool

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestPodUsageHandlerMarksReservedAndInUse(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	if name := NewPodUsageHandler(testr.New(t), nil).Name(); name != "pod-usage" {
		t.Fatalf("expected handler name pod-usage, got %s", name)
	}

	devs := []v1alpha1.GPUDevice{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "dev1",
				Annotations: map[string]string{assignmentAnnotation: "cluster-a"},
				Labels:      map[string]string{"kubernetes.io/hostname": "node1"},
			},
			Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev1", State: v1alpha1.GPUDeviceStateAssigned},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "dev2",
				Annotations: map[string]string{assignmentAnnotation: "cluster-a"},
				Labels:      map[string]string{"kubernetes.io/hostname": "node1"},
			},
			Status: v1alpha1.GPUDeviceStatus{InventoryID: "dev2", State: v1alpha1.GPUDeviceStateAssigned},
		},
	}

	pendingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "ns"},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName("cluster.gpu.deckhouse.io/cluster-a"): resource.MustParse("1"),
					},
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	runningPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "running", Namespace: "ns"},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName("cluster.gpu.deckhouse.io/cluster-a"): resource.MustParse("1"),
					},
				},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}

	objs := []client.Object{pendingPod, runningPod}
	for i := range devs {
		dev := devs[i]
		objs = append(objs, &dev)
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUDevice{}).
		WithObjects(objs...).
		Build()

	handler := NewPodUsageHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterGPUPool"},
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}

	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool failed: %v", err)
	}

	updated := &v1alpha1.GPUDevice{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev1"}, updated); err != nil {
		t.Fatalf("get dev1: %v", err)
	}
	state1 := updated.Status.State
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "dev2"}, updated); err != nil {
		t.Fatalf("get dev2: %v", err)
	}
	state2 := updated.Status.State

	if state1 == state2 {
		t.Fatalf("expected mixed states for pods (reserved and in-use), got %s and %s", state1, state2)
	}
	if (state1 != v1alpha1.GPUDeviceStateInUse && state2 != v1alpha1.GPUDeviceStateInUse) ||
		(state1 != v1alpha1.GPUDeviceStateReserved && state2 != v1alpha1.GPUDeviceStateReserved) {
		t.Fatalf("expected one device InUse and one Reserved, got %s and %s", state1, state2)
	}
}

func TestPodUsageHandlerNoClient(t *testing.T) {
	h := NewPodUsageHandler(testr.New(t), nil)
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{}); err != nil {
		t.Fatalf("expected nil client to be noop, got %v", err)
	}

	// Namespaced pool with no pods/devices should be noop too.
	nsScheme := runtime.NewScheme()
	_ = corev1.AddToScheme(nsScheme)
	_ = v1alpha1.AddToScheme(nsScheme)
	cl := fake.NewClientBuilder().WithScheme(nsScheme).Build()
	h = NewPodUsageHandler(testr.New(t), cl)
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "local", Namespace: "ns"}}); err != nil {
		t.Fatalf("expected noop with empty client, got %v", err)
	}
}

func TestPodReadyAndTotalResource(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName("gpu.deckhouse.io/pool"): resource.MustParse("1"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceName("gpu.deckhouse.io/pool"): resource.MustParse("2"),
					},
				},
			}},
		},
	}
	if total := totalResource(pod, "gpu.deckhouse.io/pool"); total != 1 {
		t.Fatalf("expected totalResource to use limits when present, got %d", total)
	}
	if podReady(pod) {
		t.Fatalf("expected podReady false without condition")
	}
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	if !podReady(pod) {
		t.Fatalf("expected podReady true with Ready condition")
	}

	// requests only in init container
	pod = &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceName("gpu.deckhouse.io/pool"): resource.MustParse("3"),
					},
				},
			}},
		},
	}
	if total := totalResource(pod, "gpu.deckhouse.io/pool"); total != 3 {
		t.Fatalf("expected totalResource to include init container requests, got %d", total)
	}
}
