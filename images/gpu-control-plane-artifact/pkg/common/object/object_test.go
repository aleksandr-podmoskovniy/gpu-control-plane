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

package object

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type failingGetClient struct {
	client.Client
	err error
}

func (c *failingGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return c.err
}

type failingDeleteClient struct {
	client.Client
	err error
}

func (c *failingDeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return c.err
}

func TestFetchObject(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	ctx := context.Background()

	t.Run("not found returns nil,nil", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		got, err := FetchObject(ctx, types.NamespacedName{Namespace: "ns", Name: "missing"}, cl, &corev1.ConfigMap{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil object, got %#v", got)
		}
	})

	t.Run("returns object", func(t *testing.T) {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cm"}}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
		got, err := FetchObject(ctx, types.NamespacedName{Namespace: "ns", Name: "cm"}, cl, &corev1.ConfigMap{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || got.GetName() != "cm" || got.GetNamespace() != "ns" {
			t.Fatalf("unexpected object: %#v", got)
		}
	})

	t.Run("propagates error", func(t *testing.T) {
		errBoom := apierrors.NewBadRequest("boom")
		got, err := FetchObject(ctx, types.NamespacedName{Namespace: "ns", Name: "cm"}, &failingGetClient{err: errBoom}, &corev1.ConfigMap{})
		if got != nil || !apierrors.IsBadRequest(err) {
			t.Fatalf("expected bad request error, got %v (obj=%#v)", err, got)
		}
	})
}

func TestDeleteObject(t *testing.T) {
	ctx := context.Background()

	t.Run("nil object is no-op", func(t *testing.T) {
		if err := DeleteObject(ctx, nil, (*corev1.ConfigMap)(nil)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("terminating object is no-op", func(t *testing.T) {
		ts := metav1.NewTime(time.Now())
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns", DeletionTimestamp: &ts}}
		if err := DeleteObject(ctx, nil, cm); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found is ignored", func(t *testing.T) {
		notFound := apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, "cm")
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}}
		if err := DeleteObject(ctx, &failingDeleteClient{err: notFound}, cm); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("propagates delete error", func(t *testing.T) {
		errBoom := apierrors.NewBadRequest("boom")
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}}
		if err := DeleteObject(ctx, &failingDeleteClient{err: errBoom}, cm); !apierrors.IsBadRequest(err) {
			t.Fatalf("expected bad request error, got %v", err)
		}
	})
}

func TestIsNilObject(t *testing.T) {
	if !isNilObject[any](nil) {
		t.Fatalf("expected nil interface to be nil")
	}
	if isNilObject[int](0) {
		t.Fatalf("expected int value to be non-nil")
	}
}
