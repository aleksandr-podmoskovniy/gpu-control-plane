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

package inventory

import (
	"context"
	"errors"
	"testing"
	"time"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestReconcileReturnsErrorWhenNodeGetFails(t *testing.T) {
	scheme := newTestScheme(t)
	boom := errors.New("node get boom")

	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	baseClient := newTestClient(scheme)
	rec.client = &delegatingClient{
		Client: baseClient,
		get: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
			return boom
		},
	}
	rec.scheme = scheme
	rec.recorder = record.NewFakeRecorder(4)

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "worker"}}); !errors.Is(err, boom) {
		t.Fatalf("expected error %v, got %v", boom, err)
	}
}

func TestReconcileReturnsErrorWhenNodeFeatureLookupFails(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-feature-error", UID: types.UID("worker-feature-error")}}
	baseClient := newTestClient(scheme, node)

	boom := errors.New("nodefeature list boom")
	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	rec.client = &delegatingClient{
		Client: baseClient,
		list: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*nfdv1alpha1.NodeFeatureList); ok {
				return boom
			}
			return baseClient.List(ctx, list, opts...)
		},
	}
	rec.scheme = scheme
	rec.recorder = record.NewFakeRecorder(4)

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); !errors.Is(err, boom) {
		t.Fatalf("expected error %v, got %v", boom, err)
	}
}

func TestReconcileReturnsErrorWhenDeviceServiceFails(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-device-error",
			UID:  types.UID("worker-device-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2203",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	boom := errors.New("device reconcile boom")

	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	rec.client = newTestClient(scheme, node)
	rec.scheme = scheme
	rec.recorder = record.NewFakeRecorder(4)
	rec.deviceService = failingDeviceService{err: boom}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); !errors.Is(err, boom) {
		t.Fatalf("expected error %v, got %v", boom, err)
	}
}

func TestReconcileReturnsErrorWhenRemoveOrphansFails(t *testing.T) {
	scheme := newTestScheme(t)
	ts := metav1.NewTime(time.Now())
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "worker-orphans-error",
			UID:               types.UID("worker-orphans-error"),
			DeletionTimestamp: &ts,
			Finalizers:        []string{"test-finalizer"},
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2203",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	boom := errors.New("orphans boom")

	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	rec.client = newTestClient(scheme, node)
	rec.scheme = scheme
	rec.recorder = record.NewFakeRecorder(4)
	rec.cleanupService = failingCleanupService{err: boom}
	rec.deviceService = fixedDeviceService{device: &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "device"}}, result: contracts.Result{}}

	if _, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); !errors.Is(err, boom) {
		t.Fatalf("expected error %v, got %v", boom, err)
	}
}
