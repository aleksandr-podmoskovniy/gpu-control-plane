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

	"github.com/go-logr/logr/testr"
	admv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

func TestGPUPoolWebhookInterfaceBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	validator := NewGPUPoolValidator(testr.New(t), cl, nil)
	defaulter := NewGPUPoolDefaulter(testr.New(t), nil)

	ctx := cradmission.NewContextWithRequest(context.Background(), cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{Namespace: "gpu-team"},
	})

	t.Run("validator-delete-noop", func(t *testing.T) {
		warnings, err := validator.ValidateDelete(ctx, &v1alpha1.GPUPool{})
		if err != nil || len(warnings) != 0 {
			t.Fatalf("expected (nil,nil), got (%v,%v)", warnings, err)
		}
	})

	t.Run("validator-create-wrong-type", func(t *testing.T) {
		if _, err := validator.ValidateCreate(ctx, &corev1.Pod{}); err == nil {
			t.Fatalf("expected type assertion error")
		}
	})

	t.Run("validator-update-wrong-old-type", func(t *testing.T) {
		if _, err := validator.ValidateUpdate(ctx, &corev1.Pod{}, &v1alpha1.GPUPool{}); err == nil {
			t.Fatalf("expected type assertion error")
		}
	})

	t.Run("validator-update-wrong-new-type", func(t *testing.T) {
		if _, err := validator.ValidateUpdate(ctx, &v1alpha1.GPUPool{}, &corev1.Pod{}); err == nil {
			t.Fatalf("expected type assertion error")
		}
	})

	t.Run("validator-admission-namespace-from-context", func(t *testing.T) {
		pool := &v1alpha1.GPUPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-a"},
			Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
		}
		if _, err := validator.ValidateCreate(ctx, pool); err != nil {
			t.Fatalf("expected create to be allowed, got %v", err)
		}

		oldPool := pool.DeepCopy()
		newPool := pool.DeepCopy()
		if _, err := validator.ValidateUpdate(ctx, oldPool, newPool); err != nil {
			t.Fatalf("expected update to be allowed, got %v", err)
		}
	})

	t.Run("validator-update-unique-name-check-fails", func(t *testing.T) {
		existing := &v1alpha1.GPUPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "other"},
			Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
		validator := NewGPUPoolValidator(testr.New(t), cl, nil)

		oldPool := &v1alpha1.GPUPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"},
			Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
		}
		newPool := oldPool.DeepCopy()
		if _, err := validator.ValidateUpdate(context.Background(), oldPool, newPool); err == nil {
			t.Fatalf("expected uniqueness validation error")
		}
	})

	t.Run("validator-update-handler-error", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		validator := NewGPUPoolValidator(testr.New(t), cl, []contracts.AdmissionHandler{
			errorAdmissionHandler{err: errors.New("boom")},
		})

		oldPool := &v1alpha1.GPUPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"},
			Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
		}
		newPool := oldPool.DeepCopy()
		if _, err := validator.ValidateUpdate(context.Background(), oldPool, newPool); err == nil {
			t.Fatalf("expected handler error to be returned")
		}
	})

	t.Run("defaulter-wrong-type", func(t *testing.T) {
		if err := defaulter.Default(ctx, &corev1.Pod{}); err == nil {
			t.Fatalf("expected type assertion error")
		}
	})
}
