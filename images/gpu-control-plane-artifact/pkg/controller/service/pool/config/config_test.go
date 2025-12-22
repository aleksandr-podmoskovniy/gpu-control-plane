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

package config

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
)

func TestConfigCheckName(t *testing.T) {
	if name := NewConfigCheckHandler(nil).Name(); name != "config-check" {
		t.Fatalf("unexpected name %q", name)
	}
}

func TestConfigCheckHandlesCollisionAndSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	clusterPool := &v1alpha1.ClusterGPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a"}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(clusterPool).Build()
	handler := NewConfigCheckHandler(client)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "pool-a",
			Namespace:  "ns",
			Generation: 1,
		},
	}
	if _, err := handler.HandlePool(context.Background(), pool); err == nil || !errors.Is(err, reconciler.ErrStopHandlerChain) {
		t.Fatalf("expected stop-chain error, got %v", err)
	}
	cond := meta.FindStatusCondition(pool.Status.Conditions, conditionConfigured)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "NameCollision" {
		t.Fatalf("expected collision condition, got %+v", cond)
	}

	// remove cluster pool, expect success condition
	_ = client.Delete(context.Background(), clusterPool)
	pool.Status.Conditions = nil
	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool returned error: %v", err)
	}
	cond = meta.FindStatusCondition(pool.Status.Conditions, conditionConfigured)
	if cond == nil || cond.Status != metav1.ConditionTrue || cond.Reason != "Configured" {
		t.Fatalf("expected configured condition, got %+v", cond)
	}
}

func TestConfigCheckSkipsWhenClientMissing(t *testing.T) {
	pool := &v1alpha1.GPUPool{}
	handler := NewConfigCheckHandler(nil)
	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool returned error: %v", err)
	}
	if len(pool.Status.Conditions) != 0 {
		t.Fatalf("conditions should stay untouched when client is nil")
	}
}

func TestConfigCheckClusterScoped(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := NewConfigCheckHandler(client)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cluster-pool",
			Generation: 2,
		},
	}
	if _, err := handler.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool returned error: %v", err)
	}
	cond := meta.FindStatusCondition(pool.Status.Conditions, conditionConfigured)
	if cond == nil || cond.Status != metav1.ConditionTrue || cond.ObservedGeneration != 2 {
		t.Fatalf("expected configured condition for cluster pool, got %+v", cond)
	}
}

func TestConfigCheckClientError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := NewConfigCheckHandler(errGetClient{Client: base})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "ns"}}
	if _, err := handler.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected error from client")
	}
}

type errGetClient struct{ client.Client }

func (errGetClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("boom")
}
