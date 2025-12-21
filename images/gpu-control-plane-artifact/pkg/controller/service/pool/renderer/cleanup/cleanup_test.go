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

package cleanup

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type deleteNthErrorClient struct {
	client.Client
	failOn int
	err    error
	count  int
}

func (c *deleteNthErrorClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	c.count++
	if c.count == c.failOn {
		return c.err
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func TestCleanupPoolResourcesErrorPaths(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	tests := []struct {
		name   string
		failOn int
	}{
		{name: "device plugin daemonset delete", failOn: 1},
		{name: "device plugin configmap delete", failOn: 2},
		{name: "mig manager daemonset delete", failOn: 3},
		{name: "mig manager configmap delete", failOn: 4},
		{name: "validator daemonset delete", failOn: 7},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := fake.NewClientBuilder().WithScheme(scheme).Build()
			cl := &deleteNthErrorClient{Client: base, failOn: tc.failOn, err: errors.New("delete error")}

			if err := PoolResources(context.Background(), cl, "ns", "alpha"); err == nil {
				t.Fatalf("expected cleanup error")
			}
		})
	}
}
