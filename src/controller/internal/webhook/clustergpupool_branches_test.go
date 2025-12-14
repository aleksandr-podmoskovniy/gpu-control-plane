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
	"net/http"
	"testing"

	"github.com/go-logr/logr/testr"
	admv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/handlers/admission"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

func TestClusterGPUPoolValidatorDecodeAndOldDecodeErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	validator := newClusterGPUPoolValidator(testr.New(t), decoder, nil, fake.NewClientBuilder().WithScheme(scheme).Build())

	resp := validator.Handle(context.Background(), cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Create,
		Object:    runtime.RawExtension{Raw: []byte{}},
	}})
	if resp.Result == nil || resp.Result.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 on decode error, got %+v", resp.Result)
	}

	validNew := []byte(`{"kind":"ClusterGPUPool","apiVersion":"gpu.deckhouse.io/v1alpha1"}`)
	resp = validator.Handle(context.Background(), cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Update,
		Object:    runtime.RawExtension{Raw: validNew},
		OldObject: runtime.RawExtension{Raw: []byte("not json")},
	}})
	if resp.Result == nil || resp.Result.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 on old decode error, got %+v", resp.Result)
	}
}

func TestClusterGPUPoolValidatorAllowsAndHandlerError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	handlers := []contracts.AdmissionHandler{admission.NewPoolValidationHandler(testr.New(t))}
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	pool := v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}
	raw, _ := json.Marshal(pool)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Create,
		Object:    runtime.RawExtension{Raw: raw},
	}}

	validator := newClusterGPUPoolValidator(testr.New(t), decoder, handlers, cl)
	if resp := validator.Handle(context.Background(), req); !resp.Allowed {
		t.Fatalf("expected allowed, got %v", resp.Result)
	}

	badHandler := errorAdmissionHandler{err: errors.New("boom")}
	validator = newClusterGPUPoolValidator(testr.New(t), decoder, []contracts.AdmissionHandler{badHandler}, cl)
	if resp := validator.Handle(context.Background(), req); resp.Allowed {
		t.Fatalf("expected denial on handler error")
	}
}

func TestClusterGPUPoolDefaulterErrorBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	defaulter := newClusterGPUPoolDefaulter(testr.New(t), decoder, nil)
	resp := defaulter.Handle(context.Background(), cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Create,
		Object:    runtime.RawExtension{Raw: []byte{}},
	}})
	if resp.Result == nil || resp.Result.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 on decode error, got %+v", resp.Result)
	}

	rawPool := v1alpha1.ClusterGPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}
	raw, _ := json.Marshal(rawPool)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Create,
		Object:    runtime.RawExtension{Raw: raw},
	}}

	defaulter = newClusterGPUPoolDefaulter(testr.New(t), decoder, []contracts.AdmissionHandler{errorAdmissionHandler{err: errors.New("boom")}})
	if resp := defaulter.Handle(context.Background(), req); resp.Allowed {
		t.Fatalf("expected denial on handler error")
	}

	defaulter = newClusterGPUPoolDefaulter(testr.New(t), decoder, nil)
	origMarshal := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { jsonMarshal = origMarshal }()
	resp = defaulter.Handle(context.Background(), req)
	if resp.Result == nil || resp.Result.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on marshal error, got %+v", resp.Result)
	}
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
