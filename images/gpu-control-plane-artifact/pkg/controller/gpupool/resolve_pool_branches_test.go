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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

type failingGetClient struct {
	client.Client
}

func (f *failingGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return apierrors.NewBadRequest("boom")
}

func TestResolvePoolByRequestBranches(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	if _, err := resolvePoolByRequest(ctx, cl, poolRequest{name: "pool", keyPrefix: "weird/"}, "ns"); err == nil {
		t.Fatalf("expected error for unknown prefix")
	}

	if _, err := resolvePoolByRequest(ctx, cl, poolRequest{name: "missing", keyPrefix: clusterPoolResourcePrefix}, "ns"); err == nil {
		t.Fatalf("expected not found error for cluster pool")
	}

	badGet := &failingGetClient{Client: cl}
	if _, err := resolvePoolByRequest(ctx, badGet, poolRequest{name: "pool", keyPrefix: clusterPoolResourcePrefix}, "ns"); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected get error for cluster pool, got %v", err)
	}
	if _, err := resolvePoolByRequest(ctx, badGet, poolRequest{name: "pool", keyPrefix: localPoolResourcePrefix}, "ns"); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected get error for namespaced pool, got %v", err)
	}
}
