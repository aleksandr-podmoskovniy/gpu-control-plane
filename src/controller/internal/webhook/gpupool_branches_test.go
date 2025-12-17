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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

type clusterPoolGetErrorClient struct {
	client.Client
	err error
}

func (c *clusterPoolGetErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*v1alpha1.ClusterGPUPool); ok {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

type listErrorClient struct {
	client.Client
	err error
}

func (c *listErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.err
}

func TestGPUPoolDefaulterMarshalError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	decoder := cradmission.NewDecoder(scheme)

	pool := v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}
	raw, _ := json.Marshal(pool)
	req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{
		Operation: admv1.Create,
		Object:    runtime.RawExtension{Raw: raw},
	}}

	defaulter := newGPUPoolDefaulter(testr.New(t), decoder, nil)
	origMarshal := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { jsonMarshal = origMarshal }()

	resp := defaulter.Handle(context.Background(), req)
	if resp.Result == nil || resp.Result.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on marshal error, got %+v", resp.Result)
	}
}

func TestValidateNamespacedPoolNameUniqueBranches(t *testing.T) {
	ctx := context.Background()

	if err := validateNamespacedPoolNameUnique(ctx, nil, nil, ""); err != nil {
		t.Fatalf("expected nil pool to be ignored: %v", err)
	}

	if err := validateNamespacedPoolNameUnique(ctx, nil, &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}, ""); err == nil {
		t.Fatalf("expected error when client is nil")
	}

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	// admissionNamespace branch + item name/namespace filters
	existingSameNS := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns1"}}
	existingOtherNS := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns2"}}
	otherName := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "ns1"}}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingSameNS, existingOtherNS, otherName).Build()

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	err := validateNamespacedPoolNameUnique(ctx, base, pool, "ns1")
	if err == nil {
		t.Fatalf("expected conflict error across namespaces")
	}

	// empty pool name is ignored
	if err := validateNamespacedPoolNameUnique(ctx, base, &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "  "}}, "ns1"); err != nil {
		t.Fatalf("expected empty name to be ignored: %v", err)
	}

	// cluster pool Get error (non-NotFound) is propagated as a wrapped error
	getErrClient := &clusterPoolGetErrorClient{Client: base, err: apierrors.NewBadRequest("boom")}
	if err := validateNamespacedPoolNameUnique(ctx, getErrClient, &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns1"}}, ""); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected wrapped get error, got %v", err)
	}

	// list error branch
	listErrClient := &listErrorClient{Client: base, err: apierrors.NewBadRequest("list failed")}
	if err := validateNamespacedPoolNameUnique(ctx, listErrClient, &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns1"}}, ""); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected list error, got %v", err)
	}
}
