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

package ops

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

type getErrorClient struct {
	client.Client
	err error
}

func (c getErrorClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return c.err
}

func TestCreateOrUpdateConfigMapAndDaemonSetBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pool",
			Namespace: "ns",
			UID:       types.UID("uid"),
		},
	}

	t.Run("configmap create", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()

		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}, Data: map[string]string{"k": "v"}}
		if err := CreateOrUpdate(context.Background(), cl, cm, pool); err != nil {
			t.Fatalf("createOrUpdate: %v", err)
		}
		got := &corev1.ConfigMap{}
		if err := cl.Get(context.Background(), client.ObjectKeyFromObject(cm), got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if !hasOwner(got, pool) {
			t.Fatalf("expected owner reference on created ConfigMap")
		}
	})

	t.Run("configmap noop when equal", func(t *testing.T) {
		existing := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}, Data: map[string]string{"k": "v"}}
		addOwner(existing, pool)

		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

		desired := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}, Data: map[string]string{"k": "v"}}
		if err := CreateOrUpdate(context.Background(), cl, desired, pool); err != nil {
			t.Fatalf("createOrUpdate: %v", err)
		}
	})

	t.Run("configmap update when differs", func(t *testing.T) {
		existing := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}, Data: map[string]string{"k": "old"}}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

		desired := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}, Data: map[string]string{"k": "new"}}
		if err := CreateOrUpdate(context.Background(), cl, desired, pool); err != nil {
			t.Fatalf("createOrUpdate: %v", err)
		}
		got := &corev1.ConfigMap{}
		if err := cl.Get(context.Background(), client.ObjectKeyFromObject(desired), got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Data["k"] != "new" || !hasOwner(got, pool) {
			t.Fatalf("expected updated data and owner, got data=%v ownerRefs=%v", got.Data, got.OwnerReferences)
		}
	})

	t.Run("daemonset create/noop/update", func(t *testing.T) {
		dsKey := client.ObjectKey{Name: "ds", Namespace: "ns"}

		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "ns"}}
		if err := CreateOrUpdate(context.Background(), cl, ds, pool); err != nil {
			t.Fatalf("createOrUpdate create: %v", err)
		}

		existing := &appsv1.DaemonSet{}
		if err := cl.Get(context.Background(), dsKey, existing); err != nil {
			t.Fatalf("get: %v", err)
		}
		if !hasOwner(existing, pool) {
			t.Fatalf("expected owner reference on created DaemonSet")
		}

		desiredNoop := existing.DeepCopy()
		if err := CreateOrUpdate(context.Background(), cl, desiredNoop, pool); err != nil {
			t.Fatalf("createOrUpdate noop: %v", err)
		}

		desiredUpdate := existing.DeepCopy()
		if desiredUpdate.Labels == nil {
			desiredUpdate.Labels = map[string]string{}
		}
		desiredUpdate.Labels["x"] = "y"
		if err := CreateOrUpdate(context.Background(), cl, desiredUpdate, pool); err != nil {
			t.Fatalf("createOrUpdate update: %v", err)
		}
	})

	t.Run("get error is returned for configmap and daemonset", func(t *testing.T) {
		base := fake.NewClientBuilder().WithScheme(scheme).Build()
		errClient := getErrorClient{Client: base, err: errors.New("boom")}

		if err := CreateOrUpdate(context.Background(), errClient, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}}, pool); err == nil {
			t.Fatalf("expected get error for ConfigMap")
		}
		if err := CreateOrUpdate(context.Background(), errClient, &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "ns"}}, pool); err == nil {
			t.Fatalf("expected get error for DaemonSet")
		}
	})

	t.Run("unsupported type errors", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		if err := CreateOrUpdate(context.Background(), cl, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "ns"}}, pool); err == nil {
			t.Fatalf("expected unsupported type error")
		}
	})
}

func TestAddOwnerSkipsCrossNamespaceAndDoesNotDuplicate(t *testing.T) {
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns", UID: "uid"}}

	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "other"}}
	addOwner(cm, pool)
	if len(cm.OwnerReferences) != 0 {
		t.Fatalf("expected owner to be skipped for cross-namespace object")
	}

	cm.Namespace = "ns"
	addOwner(cm, pool)
	addOwner(cm, pool)
	if len(cm.OwnerReferences) != 1 {
		t.Fatalf("expected no duplicate owner references, got %d", len(cm.OwnerReferences))
	}
}

func TestHasOwnerUsesClusterKindFallback(t *testing.T) {
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cm",
			Namespace: "ns",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "ClusterGPUPool",
				Name:       "pool",
			}},
		},
	}
	if !hasOwner(cm, pool) {
		t.Fatalf("expected hasOwner=true for cluster pool kind fallback")
	}
}
