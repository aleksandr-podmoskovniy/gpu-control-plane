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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

type listErrorClient struct {
	client.Client
	err error
}

func (c *listErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.err
}

func TestClusterPoolAsGPUPoolBranches(t *testing.T) {
	if clusterPoolAsGPUPool(nil) != nil {
		t.Fatalf("expected nil input to return nil")
	}

	pool := &v1alpha1.ClusterGPUPool{}
	out := clusterPoolAsGPUPool(pool)
	if out.Kind != "ClusterGPUPool" {
		t.Fatalf("expected default kind to be set, got %q", out.Kind)
	}

	pool.TypeMeta = metav1.TypeMeta{Kind: "ClusterGPUPool"}
	out = clusterPoolAsGPUPool(pool)
	if out.Kind != "ClusterGPUPool" {
		t.Fatalf("expected kind to be preserved, got %q", out.Kind)
	}
}

func TestValidateClusterPoolNameUniqueBranches(t *testing.T) {
	ctx := context.Background()

	if err := validateClusterPoolNameUnique(ctx, nil, nil); err != nil {
		t.Fatalf("expected nil pool to be ignored: %v", err)
	}

	if err := validateClusterPoolNameUnique(ctx, nil, &v1alpha1.ClusterGPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}); err == nil {
		t.Fatalf("expected nil client to error")
	}

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "ns"}}).Build()
	if err := validateClusterPoolNameUnique(ctx, base, &v1alpha1.ClusterGPUPool{ObjectMeta: metav1.ObjectMeta{Name: "   "}}); err != nil {
		t.Fatalf("expected empty name to be ignored: %v", err)
	}

	badList := &listErrorClient{Client: base, err: apierrors.NewBadRequest("list failed")}
	if err := validateClusterPoolNameUnique(ctx, badList, &v1alpha1.ClusterGPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected list error, got %v", err)
	}

	// no conflict: only non-matching pools should be ignored
	if err := validateClusterPoolNameUnique(ctx, base, &v1alpha1.ClusterGPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}); err != nil {
		t.Fatalf("expected no conflict, got %v", err)
	}
}
