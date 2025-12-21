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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

type listErrorClient struct{ client.Client }

func (c listErrorClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return errors.New("list error")
}

func TestClusterPoolAsGPUPoolNilAndKind(t *testing.T) {
	if clusterPoolAsGPUPool(nil) != nil {
		t.Fatalf("expected nil for nil pool")
	}

	cluster := &v1alpha1.ClusterGPUPool{TypeMeta: metav1.TypeMeta{Kind: "ClusterGPUPool"}, ObjectMeta: metav1.ObjectMeta{Name: "p"}}
	out := clusterPoolAsGPUPool(cluster)
	if out == nil || out.Kind != "ClusterGPUPool" {
		t.Fatalf("unexpected conversion: %+v", out)
	}
}

func TestValidateClusterPoolNameUniqueAdditionalBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	if err := validateClusterPoolNameUnique(context.Background(), fake.NewClientBuilder().WithScheme(scheme).Build(), nil); err != nil {
		t.Fatalf("expected nil for nil pool, got %v", err)
	}
	if err := validateClusterPoolNameUnique(context.Background(), fake.NewClientBuilder().WithScheme(scheme).Build(), &v1alpha1.ClusterGPUPool{}); err != nil {
		t.Fatalf("expected nil for empty name, got %v", err)
	}

	cluster := &v1alpha1.ClusterGPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	if err := validateClusterPoolNameUnique(context.Background(), nil, cluster); err == nil {
		t.Fatalf("expected error for nil client")
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(&v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "ns"}}).
		Build()
	if err := validateClusterPoolNameUnique(context.Background(), cl, cluster); err != nil {
		t.Fatalf("expected no conflicts, got %v", err)
	}

	errClient := listErrorClient{Client: cl}
	if err := validateClusterPoolNameUnique(context.Background(), errClient, cluster); err == nil || !strings.Contains(err.Error(), "list GPUPools") {
		t.Fatalf("expected list error, got %v", err)
	}
}
