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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type namespaceGetErrorClient struct {
	client.Client
	err error
}

func (c *namespaceGetErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*corev1.Namespace); ok {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func TestRequireGPUEnabledNamespaceAdditionalBranches(t *testing.T) {
	t.Run("no-client-is-noop", func(t *testing.T) {
		if err := requireGPUEnabledNamespace(context.Background(), nil, ""); err != nil {
			t.Fatalf("expected nil error when client is nil, got %v", err)
		}
	})

	t.Run("empty-namespace-is-error", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()

		if err := requireGPUEnabledNamespace(context.Background(), cl, "   "); err == nil {
			t.Fatalf("expected error for empty namespace")
		}
	})

	t.Run("get-error-is-wrapped", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		base := fake.NewClientBuilder().WithScheme(scheme).Build()
		cl := &namespaceGetErrorClient{Client: base, err: apierrors.NewBadRequest("boom")}

		err := requireGPUEnabledNamespace(context.Background(), cl, "gpu-ns")
		if err == nil {
			t.Fatalf("expected error to be returned")
		}
		if !apierrors.IsBadRequest(err) {
			t.Fatalf("expected bad request to be detectable, got %v", err)
		}
		if !strings.Contains(err.Error(), "get namespace") {
			t.Fatalf("expected wrapped message, got %v", err)
		}
	})
}
