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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestResolvePoolByRequestErrorBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	base := fake.NewClientBuilder().WithScheme(scheme).Build()

	if _, err := resolvePoolByRequest(context.Background(), getErrorClient{Client: base, err: errors.New("boom")}, poolRequest{name: "a", keyPrefix: clusterPoolResourcePrefix}, ""); err == nil {
		t.Fatalf("expected cluster FetchObject error")
	}
	if _, err := resolvePoolByRequest(context.Background(), base, poolRequest{name: "missing", keyPrefix: clusterPoolResourcePrefix}, ""); err == nil {
		t.Fatalf("expected ClusterGPUPool not found error")
	}

	if _, err := resolvePoolByRequest(context.Background(), getErrorClient{Client: base, err: errors.New("boom")}, poolRequest{name: "a", keyPrefix: localPoolResourcePrefix}, "ns"); err == nil {
		t.Fatalf("expected namespaced FetchObject error")
	}

	if _, err := resolvePoolByRequest(context.Background(), base, poolRequest{name: "a", keyPrefix: "weird/"}, "ns"); err == nil {
		t.Fatalf("expected unknown prefix error")
	}
}
