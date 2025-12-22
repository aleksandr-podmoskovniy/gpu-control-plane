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

package clustergpupool

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add gpu scheme: %v", err)
	}
	return scheme
}

func TestClusterGPUPoolUsageReconcileUpdatesUsedAvailable(t *testing.T) {
	scheme := newScheme(t)

	pool := &v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1},
		},
	}

	resourceName := corev1.ResourceName("cluster.gpu.deckhouse.io/cluster-a")
	podLabels := map[string]string{
		poolcommon.PoolNameKey:  "cluster-a",
		poolcommon.PoolScopeKey: poolcommon.PoolScopeCluster,
	}

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1", Labels: podLabels},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{resourceName: resource.MustParse("1")},
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "ns2", Labels: podLabels},
		Spec: corev1.PodSpec{
			NodeName: "node2",
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{resourceName: resource.MustParse("1")},
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	unscheduled := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unscheduled", Namespace: "ns3", Labels: podLabels},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "c",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{resourceName: resource.MustParse("1")},
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	cl := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.ClusterGPUPool{}).
		WithObjects(pool, pod1, pod2, unscheduled).
		Build()

	r := NewReconciler(testr.New(t), config.ControllerConfig{Workers: 1}, nil)
	r.client = cl

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "cluster-a"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &v1alpha1.ClusterGPUPool{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(pool), got); err != nil {
		t.Fatalf("get pool: %v", err)
	}
	if got.Status.Capacity.Used != 2 {
		t.Fatalf("expected used=2, got %d", got.Status.Capacity.Used)
	}
	if got.Status.Capacity.Available != 0 {
		t.Fatalf("expected available=0, got %d", got.Status.Capacity.Available)
	}
}
