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
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestValidateNamespacedPoolNameUniqueBranches(t *testing.T) {
	if err := validateNamespacedPoolNameUnique(context.Background(), nil, nil, ""); err != nil {
		t.Fatalf("expected nil for nil pool, got %v", err)
	}

	if err := validateNamespacedPoolNameUnique(context.Background(), nil, &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "a"}}, "ns"); err == nil {
		t.Fatalf("expected client not configured error")
	}

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	base := fake.NewClientBuilder().WithScheme(scheme).Build()

	if err := validateNamespacedPoolNameUnique(context.Background(), base, &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "  "}}, "ns"); err != nil {
		t.Fatalf("expected nil for empty name, got %v", err)
	}

	if err := validateNamespacedPoolNameUnique(context.Background(), getErrorClient{Client: base, err: errors.New("boom")}, &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "a"}}, "ns"); err == nil {
		t.Fatalf("expected ClusterGPUPool check error")
	}

	cluster := &v1alpha1.ClusterGPUPool{ObjectMeta: metav1.ObjectMeta{Name: "a"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	if err := validateNamespacedPoolNameUnique(context.Background(), cl, &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"}}, ""); err == nil {
		t.Fatalf("expected conflict with ClusterGPUPool")
	}

	if err := validateNamespacedPoolNameUnique(context.Background(), listErrorClient{Client: base, err: errors.New("list error")}, &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"}}, ""); err == nil {
		t.Fatalf("expected list error")
	}

	listClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "ns"}},
		&v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"}},
		&v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "other"}},
	).Build()

	if err := validateNamespacedPoolNameUnique(context.Background(), listClient, &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "a"}}, "ns"); err == nil {
		t.Fatalf("expected non-unique name error")
	}
}
