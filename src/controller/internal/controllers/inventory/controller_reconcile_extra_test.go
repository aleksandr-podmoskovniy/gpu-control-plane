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
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestReconcileNodeInventoryNotFoundNoDevices(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	node := &corev1.Node{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		WithObjects(node).
		Build()
	r := &Reconciler{client: cl, scheme: scheme, log: testr.New(t), recorder: record.NewFakeRecorder(5)}

	if err := r.reconcileNodeInventory(context.Background(), node, nodeSnapshot{}, nil, ManagedNodesPolicy{}, nodeDetection{}); err != nil {
		t.Fatalf("expected nil when inventory not found and no devices: %v", err)
	}
	inv := &v1alpha1.GPUNodeInventory{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: node.Name}, inv); !apierrors.IsNotFound(err) {
		t.Fatalf("inventory should not be created, got %v", err)
	}
}

func TestReconcileNodeInventoryCreatesWhenDevicesPresent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	node := &corev1.Node{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
		ObjectMeta: metav1.ObjectMeta{Name: "node1", UID: "uid-1"},
	}
	device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev1"}, Status: v1alpha1.GPUDeviceStatus{InventoryID: "inv1"}}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		WithObjects(node).
		Build()
	r := &Reconciler{client: cl, scheme: scheme, log: testr.New(t), recorder: record.NewFakeRecorder(5)}

	if err := r.reconcileNodeInventory(context.Background(), node, nodeSnapshot{}, []*v1alpha1.GPUDevice{device}, ManagedNodesPolicy{}, nodeDetection{}); err != nil {
		t.Fatalf("expected creation when devices present: %v", err)
	}
	inv := &v1alpha1.GPUNodeInventory{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: node.Name}, inv); err != nil {
		t.Fatalf("inventory should be created: %v", err)
	}
}

func TestReconcileNodeInventorySetsPCIFromDetection(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	node := &corev1.Node{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
		ObjectMeta: metav1.ObjectMeta{Name: "node1", UID: "uid-1"},
	}
	inv := &v1alpha1.GPUNodeInventory{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "GPUNodeInventory"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
		},
	}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "dev1",
			Labels: map[string]string{deviceIndexLabelKey: "0"},
		},
		Status: v1alpha1.GPUDeviceStatus{
			InventoryID: "inv1",
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.GPUNodeInventory{}).
		WithObjects(node, inv, device).
		Build()

	r := &Reconciler{client: cl, scheme: scheme, log: testr.New(t), recorder: record.NewFakeRecorder(5)}
	snapshot := nodeSnapshot{
		Devices: []deviceSnapshot{{Index: "0"}},
	}
	detections := nodeDetection{
		byIndex: map[string]detectGPUEntry{
			"0": {PCI: detectGPUPCI{Address: "0000:01:00.0"}},
		},
	}

	if err := r.reconcileNodeInventory(context.Background(), node, snapshot, []*v1alpha1.GPUDevice{device}, ManagedNodesPolicy{}, detections); err != nil {
		t.Fatalf("reconcileNodeInventory failed: %v", err)
	}

	updated := &v1alpha1.GPUNodeInventory{}
	_ = cl.Get(context.Background(), client.ObjectKey{Name: node.Name}, updated)
	if len(updated.Status.Devices) == 0 || updated.Status.Devices[0].PCI.Address != "0000:01:00.0" {
		t.Fatalf("expected PCI address populated from detections, got %+v", updated.Status.Devices)
	}
}
