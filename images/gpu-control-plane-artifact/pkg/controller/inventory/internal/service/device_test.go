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

package service

import (
	"context"
	"errors"
	"testing"

	promdto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	invconsts "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/consts"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	invmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics/inventory"
)

type namedErrorHandler struct {
	name string
	err  error
}

func (h namedErrorHandler) Name() string { return h.name }

func (h namedErrorHandler) HandleDevice(context.Context, *v1alpha1.GPUDevice) (contracts.Result, error) {
	return contracts.Result{}, h.err
}

type delegatingClient struct {
	client.Client
	patch func(context.Context, client.Object, client.Patch, ...client.PatchOption) error
}

func (d *delegatingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if d.patch != nil {
		return d.patch(ctx, obj, patch, opts...)
	}
	return d.Client.Patch(ctx, obj, patch, opts...)
}

func labelsMatch(metric *promdto.Metric, expected map[string]string) bool {
	for name, want := range expected {
		found := false
		for _, pair := range metric.Label {
			if pair.GetName() != name {
				continue
			}
			found = true
			if pair.GetValue() != want {
				return false
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func findMetric(t *testing.T, name string, labels map[string]string) (*promdto.Metric, bool) {
	t.Helper()

	families, err := crmetrics.Registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			if labelsMatch(metric, labels) {
				return metric, true
			}
		}
		return nil, false
	}

	return nil, false
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add gpu scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	return scheme
}

func newTestClient(t *testing.T, scheme *runtime.Scheme, objs ...client.Object) client.Client {
	t.Helper()

	builder := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha1.GPUDevice{}, &v1alpha1.GPUNodeState{})

	builder = builder.WithIndex(&v1alpha1.GPUDevice{}, invconsts.DeviceNodeIndexKey, func(obj client.Object) []string {
		device, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || device.Status.NodeName == "" {
			return nil
		}
		return []string{device.Status.NodeName}
	})

	return builder.Build()
}

func TestDeviceServiceInvokeHandlersIncrementsErrorMetric(t *testing.T) {
	handler := namedErrorHandler{name: "error-" + t.Name(), err: errors.New("boom")}

	svc := &DeviceService{handlers: []contracts.InventoryHandler{handler}}
	if _, err := svc.invokeHandlers(context.Background(), &v1alpha1.GPUDevice{}); err == nil {
		t.Fatalf("expected handler error")
	}

	metric, ok := findMetric(t, invmetrics.InventoryHandlerErrorsTotal, map[string]string{"handler": handler.name})
	if !ok || metric.Counter == nil || metric.Counter.GetValue() != 1 {
		value := 0.0
		if ok && metric.Counter != nil {
			value = metric.Counter.GetValue()
		}
		t.Fatalf("expected handler errors counter=1, got %f (present=%t)", value, ok)
	}
}

func TestEnsureDeviceMetadataUpdatesLabelsAndOwner(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "metadata-node",
			UID:  types.UID("metadata-node"),
		},
	}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "metadata-device",
		},
	}

	cl := newTestClient(t, scheme, node, device)
	svc := &DeviceService{client: cl, scheme: scheme}

	changed, err := svc.ensureDeviceMetadata(context.Background(), node, device, invstate.DeviceSnapshot{Index: "1"})
	if err != nil {
		t.Fatalf("ensureDeviceMetadata returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected metadata to change")
	}
	if device.Labels[invconsts.DeviceNodeLabelKey] != node.Name {
		t.Fatalf("device node label not set: %v", device.Labels)
	}
	if device.Labels[invconsts.DeviceIndexLabelKey] != "1" {
		t.Fatalf("device index label not set: %v", device.Labels)
	}
	if len(device.OwnerReferences) != 1 || device.OwnerReferences[0].Name != node.Name || device.OwnerReferences[0].Kind != "Node" {
		t.Fatalf("expected owner reference to node, got %+v", device.OwnerReferences)
	}
}

func TestEnsureDeviceMetadataNoChanges(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unchanged-node",
			UID:  types.UID("unchanged"),
		},
	}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unchanged-device",
			Labels: map[string]string{
				invconsts.DeviceNodeLabelKey:  node.Name,
				invconsts.DeviceIndexLabelKey: "0",
			},
		},
	}
	if err := controllerutil.SetOwnerReference(node, device, scheme); err != nil {
		t.Fatalf("preparing owner reference: %v", err)
	}

	svc := &DeviceService{
		client: newTestClient(t, scheme, node, device),
		scheme: scheme,
	}

	changed, err := svc.ensureDeviceMetadata(context.Background(), node, device, invstate.DeviceSnapshot{Index: "0"})
	if err != nil {
		t.Fatalf("ensureDeviceMetadata returned error: %v", err)
	}
	if changed {
		t.Fatalf("expected metadata to remain unchanged")
	}
}

func TestEnsureDeviceMetadataPatchError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "metadata-error",
			UID:  types.UID("metadata-error"),
		},
	}
	device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "metadata-device"}}

	base := newTestClient(t, scheme, node, device)
	cl := &delegatingClient{
		Client: base,
		patch: func(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
			return errors.New("metadata patch failed")
		},
	}

	svc := &DeviceService{client: cl, scheme: scheme}
	_, err := svc.ensureDeviceMetadata(context.Background(), node, device, invstate.DeviceSnapshot{Index: "0"})
	if err == nil {
		t.Fatalf("expected patch error from ensureDeviceMetadata")
	}
}

func TestEnsureDeviceMetadataOwnerReferenceError(t *testing.T) {
	scheme := runtime.NewScheme()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-meta-error"}}
	device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "worker-meta-error"}}

	svc := &DeviceService{
		client: newTestClient(t, newTestScheme(t), node, device),
		scheme: scheme,
	}

	changed, err := svc.ensureDeviceMetadata(context.Background(), node, device, invstate.DeviceSnapshot{Index: "0"})
	if err == nil {
		t.Fatal("expected owner reference error")
	}
	if changed {
		t.Fatal("expected changed=false on error")
	}
}
