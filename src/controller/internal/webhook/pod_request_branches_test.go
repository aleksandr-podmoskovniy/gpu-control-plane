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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestDecodePodRequestMarshalError(t *testing.T) {
	origMarshal := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { jsonMarshal = origMarshal }()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}}
	req := cradmission.Request{AdmissionRequest: cradmission.Request{}.AdmissionRequest}
	req.Object = runtime.RawExtension{Object: pod}

	if _, _, err := decodePodRequest(req); err == nil {
		t.Fatalf("expected marshal error")
	}
}

func TestRequireGPUEnabledNamespaceBranches(t *testing.T) {
	ctx := context.Background()

	// nil client is always a no-op (including empty namespace).
	if err := requireGPUEnabledNamespace(ctx, nil, ""); err != nil {
		t.Fatalf("expected nil client to ignore namespace validation: %v", err)
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	if err := requireGPUEnabledNamespace(ctx, cl, "   "); err == nil {
		t.Fatalf("expected empty namespace to error")
	}

	enabled := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gpu-ns", Labels: map[string]string{gpuEnabledLabelKey: "true"}}}
	cl = fake.NewClientBuilder().WithScheme(scheme).WithObjects(enabled).Build()
	if err := requireGPUEnabledNamespace(ctx, cl, "gpu-ns"); err != nil {
		t.Fatalf("expected enabled namespace to pass: %v", err)
	}

	// not found branch
	if err := requireGPUEnabledNamespace(ctx, cl, "missing"); err == nil {
		t.Fatalf("expected missing namespace to error")
	}
}
