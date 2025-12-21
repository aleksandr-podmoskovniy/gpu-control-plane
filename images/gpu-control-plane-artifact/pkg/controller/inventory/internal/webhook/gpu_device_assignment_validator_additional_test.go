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
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
)

func TestGPUDeviceAssignmentValidatorValidateDeleteNoop(t *testing.T) {
	validator := NewGPUDeviceAssignmentValidator(testr.New(t), nil)
	warnings, err := validator.ValidateDelete(context.Background(), &v1alpha1.GPUDevice{})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("expected (nil,nil), got (%v,%v)", warnings, err)
	}
}

func TestGPUDeviceAssignmentValidatorRejectsWrongTypes(t *testing.T) {
	validator := NewGPUDeviceAssignmentValidator(testr.New(t), nil)

	if _, err := validator.ValidateCreate(context.Background(), &corev1.Pod{}); err == nil {
		t.Fatalf("expected type assertion error on create")
	}

	if _, err := validator.ValidateUpdate(context.Background(), &corev1.Pod{}, &v1alpha1.GPUDevice{}); err == nil {
		t.Fatalf("expected type assertion error on update old object")
	}
	if _, err := validator.ValidateUpdate(context.Background(), &v1alpha1.GPUDevice{}, &corev1.Pod{}); err == nil {
		t.Fatalf("expected type assertion error on update new object")
	}
}

func TestGPUDeviceAssignmentValidatorUpdateRejectsBothAssignments(t *testing.T) {
	validator := NewGPUDeviceAssignmentValidator(testr.New(t), nil)

	oldDevice := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev"}}
	newDevice := oldDevice.DeepCopy()
	newDevice.Annotations = map[string]string{
		namespacedAssignmentAnnotation: "pool-a",
		clusterAssignmentAnnotation:    "cluster-a",
	}

	if _, err := validator.ValidateUpdate(context.Background(), oldDevice, newDevice); err == nil {
		t.Fatalf("expected denial when both assignment annotations are set")
	}
}

func TestGPUDeviceAssignmentValidatorValidateAssignmentPreconditions(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	t.Run("client-not-configured", func(t *testing.T) {
		validator := NewGPUDeviceAssignmentValidator(testr.New(t), nil)
		device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Annotations: map[string]string{namespacedAssignmentAnnotation: "pool-a"},
		}}
		if _, err := validator.ValidateCreate(context.Background(), device); err == nil {
			t.Fatalf("expected error without webhook client")
		}
	})

	t.Run("ignored-device", func(t *testing.T) {
		validator := NewGPUDeviceAssignmentValidator(testr.New(t), fake.NewClientBuilder().WithScheme(scheme).Build())
		device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{
			Name:        "dev",
			Labels:      map[string]string{"gpu.deckhouse.io/ignore": "true"},
			Annotations: map[string]string{namespacedAssignmentAnnotation: "pool-a"},
		}}
		if _, err := validator.ValidateCreate(context.Background(), device); err == nil {
			t.Fatalf("expected denial for ignored device")
		}
	})

	t.Run("state-not-ready", func(t *testing.T) {
		validator := NewGPUDeviceAssignmentValidator(testr.New(t), fake.NewClientBuilder().WithScheme(scheme).Build())
		device := &v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "dev",
				Annotations: map[string]string{namespacedAssignmentAnnotation: "pool-a"},
			},
			Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStatePendingAssignment},
		}
		if _, err := validator.ValidateCreate(context.Background(), device); err == nil {
			t.Fatalf("expected denial for non-ready state")
		}
	})

	t.Run("inventory-incomplete", func(t *testing.T) {
		validator := NewGPUDeviceAssignmentValidator(testr.New(t), fake.NewClientBuilder().WithScheme(scheme).Build())
		device := &v1alpha1.GPUDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "dev",
				Annotations: map[string]string{namespacedAssignmentAnnotation: "pool-a"},
			},
			Status: v1alpha1.GPUDeviceStatus{State: v1alpha1.GPUDeviceStateReady},
		}
		if _, err := validator.ValidateCreate(context.Background(), device); err == nil {
			t.Fatalf("expected denial for incomplete inventory")
		}
	})
}

func TestResolveNamespacedPoolByNameUsesIndexWhenPresent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	poolObj := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-a", Namespace: "gpu-team"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(poolObj).
		WithIndex(&v1alpha1.GPUPool{}, indexer.GPUPoolNameField, func(obj client.Object) []string {
			poolObj, ok := obj.(*v1alpha1.GPUPool)
			if !ok || poolObj.Name == "" {
				return nil
			}
			return []string{poolObj.Name}
		}).
		Build()

	found, err := resolveNamespacedPoolByName(context.Background(), cl, "pool-a")
	if err != nil {
		t.Fatalf("resolveNamespacedPoolByName: %v", err)
	}
	if found == nil || found.Name != "pool-a" {
		t.Fatalf("unexpected pool: %+v", found)
	}
}

func TestIsMissingIndexErrorNil(t *testing.T) {
	if isMissingIndexError(nil) {
		t.Fatalf("expected nil error to not be treated as missing index")
	}
}

type listMissingIndexThenErrorClient struct {
	client.Client
}

func (c *listMissingIndexThenErrorClient) List(_ context.Context, _ client.ObjectList, opts ...client.ListOption) error {
	if len(opts) > 0 {
		return errors.New(`no index with name "metadata.name" has been registered`)
	}
	return apierrors.NewBadRequest("boom")
}

func TestResolveNamespacedPoolByNamePropagatesFallbackListError(t *testing.T) {
	_, err := resolveNamespacedPoolByName(context.Background(), &listMissingIndexThenErrorClient{}, "pool-a")
	if err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected bad request error, got %v", err)
	}
}

func TestMatchesDeviceSelectorNilDevice(t *testing.T) {
	if matchesDeviceSelector(nil, nil) {
		t.Fatalf("expected nil device to not match selector")
	}
}

func TestResolveNamespacedPoolByNameEmptyName(t *testing.T) {
	_, err := resolveNamespacedPoolByName(context.Background(), &listMissingIndexThenErrorClient{}, " ")
	if err == nil {
		t.Fatalf("expected error for empty pool name")
	}
}

func TestResolveNamespacedPoolByNameNilClient(t *testing.T) {
	_, err := resolveNamespacedPoolByName(context.Background(), nil, "pool-a")
	if err == nil {
		t.Fatalf("expected error for nil client")
	}
}

type listByNameErrorClient struct {
	client.Client
	err error
}

func (c *listByNameErrorClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return c.err
}

func TestResolveNamespacedPoolByNamePropagatesIndexedListError(t *testing.T) {
	boom := errors.New("boom")
	_, err := resolveNamespacedPoolByName(context.Background(), &listByNameErrorClient{err: boom}, "pool-a")
	if err == nil || !errors.Is(err, boom) || !strings.Contains(err.Error(), "list GPUPools by name") {
		t.Fatalf("expected wrapped list error, got %v", err)
	}
}
