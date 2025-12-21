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
	"encoding/json"
	"errors"
	"testing"

	"github.com/go-logr/logr/testr"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	pooladmission "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/admission"
)

type stubAdmissionHandler struct {
	name  string
	calls int
	err   error
}

func (s *stubAdmissionHandler) Name() string { return s.name }

func (s *stubAdmissionHandler) SyncPool(_ context.Context, _ *v1alpha1.GPUPool) (contracts.Result, error) {
	s.calls++
	return contracts.Result{}, s.err
}

func TestClusterGPUPoolDefaulterAddsDefaults(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	handlers := []contracts.AdmissionHandler{pooladmission.NewPoolValidationHandler(testr.New(t))}

	rawPool := v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
		},
	}
	rawBytes, _ := json.Marshal(rawPool)

	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Operation: admv1.Create,
			Object:    runtime.RawExtension{Raw: rawBytes},
		},
	}

	defaulter := cradmission.WithCustomDefaulter(scheme, &v1alpha1.ClusterGPUPool{}, NewClusterGPUPoolDefaulter(testr.New(t), handlers))
	resp := defaulter.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed response, got denied: %v", resp.Result)
	}
	if len(resp.Patches) == 0 {
		t.Fatalf("expected patches with defaults")
	}
}

func TestClusterGPUPoolValidatorRejectsImmutableChanges(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	handlers := []contracts.AdmissionHandler{pooladmission.NewPoolValidationHandler(testr.New(t))}

	oldPool := v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Provider: "Nvidia",
			Backend:  "DevicePlugin",
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 1},
		},
	}
	newPool := oldPool.DeepCopy()
	newPool.Spec.Backend = "DRA"

	oldRaw, _ := json.Marshal(oldPool)
	newRaw, _ := json.Marshal(newPool)

	req := cradmission.Request{
		AdmissionRequest: admv1.AdmissionRequest{
			Operation: admv1.Update,
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	validator := cradmission.WithCustomValidator(scheme, &v1alpha1.ClusterGPUPool{}, NewClusterGPUPoolValidator(testr.New(t), cl, handlers))
	resp := validator.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatalf("expected immutable change to be denied")
	}
}

func TestClusterGPUPoolValidatorRejectsNameConflictWithNamespacedPool(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	handlers := []contracts.AdmissionHandler{pooladmission.NewPoolValidationHandler(testr.New(t))}

	existing := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "gpu-team"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	pool := v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}
	raw, _ := json.Marshal(pool)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Create,
		Object:    runtime.RawExtension{Raw: raw},
	}}

	validator := cradmission.WithCustomValidator(scheme, &v1alpha1.ClusterGPUPool{}, NewClusterGPUPoolValidator(testr.New(t), cl, handlers))
	if resp := validator.Handle(context.Background(), req); resp.Allowed {
		t.Fatalf("expected denial due to name conflict with GPUPool")
	}
}

func TestClusterGPUPoolValidator_AllowsCreateAndUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	handler := &stubAdmissionHandler{name: "stub"}
	validator := NewClusterGPUPoolValidator(testr.New(t), cl, []contracts.AdmissionHandler{handler})

	clusterPool := &v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}

	if _, err := validator.ValidateCreate(context.Background(), clusterPool); err != nil {
		t.Fatalf("expected create to be allowed, got %v", err)
	}

	oldPool := clusterPool.DeepCopy()
	newPool := clusterPool.DeepCopy()
	if _, err := validator.ValidateUpdate(context.Background(), oldPool, newPool); err != nil {
		t.Fatalf("expected update to be allowed, got %v", err)
	}

	if handler.calls != 2 {
		t.Fatalf("expected handler to be called twice, got %d", handler.calls)
	}
}

func TestClusterGPUPoolValidator_TypeErrors(t *testing.T) {
	validator := NewClusterGPUPoolValidator(testr.New(t), nil, nil)

	if _, err := validator.ValidateCreate(context.Background(), &v1alpha1.GPUPool{}); err == nil {
		t.Fatalf("expected create to reject unexpected object type")
	}

	if _, err := validator.ValidateUpdate(context.Background(), &v1alpha1.GPUPool{}, &v1alpha1.ClusterGPUPool{}); err == nil {
		t.Fatalf("expected update to reject unexpected old object type")
	}

	if _, err := validator.ValidateUpdate(context.Background(), &v1alpha1.ClusterGPUPool{}, &v1alpha1.GPUPool{}); err == nil {
		t.Fatalf("expected update to reject unexpected new object type")
	}
}

func TestClusterGPUPoolValidator_PropagatesNameCheckErrors(t *testing.T) {
	clusterPool := &v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}

	validator := NewClusterGPUPoolValidator(testr.New(t), nil, nil)
	if _, err := validator.ValidateCreate(context.Background(), clusterPool); err == nil {
		t.Fatalf("expected create to fail when client is nil")
	}

	if _, err := validator.ValidateUpdate(context.Background(), clusterPool, clusterPool.DeepCopy()); err == nil {
		t.Fatalf("expected update to fail when client is nil")
	}
}

func TestClusterGPUPoolValidator_PropagatesHandlerErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	handlerErr := errors.New("handler failed")
	validator := NewClusterGPUPoolValidator(testr.New(t), cl, []contracts.AdmissionHandler{&stubAdmissionHandler{name: "stub", err: handlerErr}})

	clusterPool := &v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}

	if _, err := validator.ValidateCreate(context.Background(), clusterPool); err == nil {
		t.Fatalf("expected create to propagate handler error")
	}

	oldPool := clusterPool.DeepCopy()
	newPool := clusterPool.DeepCopy()
	if _, err := validator.ValidateUpdate(context.Background(), oldPool, newPool); err == nil {
		t.Fatalf("expected update to propagate handler error")
	}
}

func TestClusterGPUPoolValidator_RejectsNameConflictOnUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	existing := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "gpu-team"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	validator := NewClusterGPUPoolValidator(testr.New(t), cl, nil)
	clusterPool := &v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}

	if _, err := validator.ValidateUpdate(context.Background(), clusterPool.DeepCopy(), clusterPool.DeepCopy()); err == nil {
		t.Fatalf("expected update to be denied due to name conflict with GPUPool")
	}
}

func TestClusterGPUPoolValidator_ValidateDeleteNoop(t *testing.T) {
	validator := NewClusterGPUPoolValidator(testr.New(t), nil, nil)
	if warnings, err := validator.ValidateDelete(context.Background(), &v1alpha1.ClusterGPUPool{}); err != nil || warnings != nil {
		t.Fatalf("expected delete to be ignored, got warnings=%v err=%v", warnings, err)
	}
}

func TestClusterGPUPoolDefaulter_ErrorsOnWrongType(t *testing.T) {
	defaulter := NewClusterGPUPoolDefaulter(testr.New(t), nil)
	if err := defaulter.Default(context.Background(), &v1alpha1.GPUPool{}); err == nil {
		t.Fatalf("expected wrong type to be rejected")
	}
}

func TestClusterGPUPoolDefaulter_PropagatesHandlerError(t *testing.T) {
	handlerErr := errors.New("sync fail")
	defaulter := NewClusterGPUPoolDefaulter(testr.New(t), []contracts.AdmissionHandler{&stubAdmissionHandler{name: "stub", err: handlerErr}})

	if err := defaulter.Default(context.Background(), &v1alpha1.ClusterGPUPool{}); err == nil {
		t.Fatalf("expected handler error")
	}
}
