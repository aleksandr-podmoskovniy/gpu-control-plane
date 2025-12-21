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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestReconcileReturnsDeleteError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-delete-error",
			UID:  types.UID("node-delete-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2230",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	primary := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-delete-error-0-10de-2230",
			Labels: map[string]string{deviceNodeLabelKey: "worker-delete-error", deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{NodeName: "worker-delete-error"},
	}
	orphan := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-delete-error-orphan",
			Labels: map[string]string{deviceNodeLabelKey: "worker-delete-error", deviceIndexLabelKey: "99"},
		},
		Status: v1alpha1.GPUDeviceStatus{NodeName: "worker-delete-error"},
	}

	baseClient := newTestClient(scheme, node, primary, orphan)
	delErr := errors.New("delete failure")
	client := &delegatingClient{
		Client: baseClient,
		delete: func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
			if dev, ok := obj.(*v1alpha1.GPUDevice); ok && dev.Name == orphan.Name {
				return delErr
			}
			return baseClient.Delete(ctx, obj, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	if _, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
		t.Fatalf("expected reconcile to ignore delete error when node is active, got %v", err)
	}
}

func TestReconcileReturnsDeviceListError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-list-error",
			UID:  types.UID("node-list-error"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-list-error"},
	}

	baseClient := newTestClient(scheme, node, feature)
	listErr := errors.New("list failure")
	client := &delegatingClient{
		Client: baseClient,
		list: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*v1alpha1.GPUDeviceList); ok {
				return listErr
			}
			return baseClient.List(ctx, list, opts...)
		},
	}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if !errors.Is(err, listErr) {
		t.Fatalf("expected device list error, got %v", err)
	}
}
