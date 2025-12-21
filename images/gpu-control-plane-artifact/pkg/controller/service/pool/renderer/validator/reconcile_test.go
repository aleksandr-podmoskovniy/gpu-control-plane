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

package validator

import (
	"context"
	"errors"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/deps"
)

func TestReconcileErrorPaths(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "ns"}}

	if err := Reconcile(context.Background(), deps.Deps{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Config: config.RenderConfig{Namespace: "ns", ValidatorImage: ""}}, pool); err == nil {
		t.Fatalf("expected validator image not configured error")
	}

	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	cl := &createNthErrorClient{Client: base, failOn: 1, err: errors.New("create error")}
	d := deps.Deps{Client: cl, Config: config.RenderConfig{Namespace: "ns", ValidatorImage: "val:tag"}}

	if err := Reconcile(context.Background(), d, pool); err == nil || !strings.Contains(err.Error(), "reconcile validator DaemonSet") {
		t.Fatalf("expected reconcile validator DaemonSet error, got %v", err)
	}
}

type createNthErrorClient struct {
	client.Client
	failOn int
	err    error

	calls int
}

func (c *createNthErrorClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.calls++
	if c.calls == c.failOn {
		return c.err
	}
	return c.Client.Create(ctx, obj, opts...)
}
