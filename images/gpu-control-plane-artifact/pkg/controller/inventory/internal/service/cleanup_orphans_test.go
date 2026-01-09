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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
)

func TestCleanupServiceRemoveOrphansNoop(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-orphans-noop"}}
	base := newTestClient(t, scheme, node)

	called := false
	cl := &cleanupDelegatingClient{
		Client: base,
		delete: func(context.Context, client.Object, ...client.DeleteOption) error {
			called = true
			return errors.New("unexpected delete")
		},
	}

	svc := NewCleanupService(cl, nil)

	if err := svc.RemoveOrphans(context.Background(), node, map[string]struct{}{}); err != nil {
		t.Fatalf("RemoveOrphans returned error: %v", err)
	}
	if called {
		t.Fatalf("expected no delete calls")
	}
}

func TestCleanupServiceRemoveOrphansDeletesDeviceAndEmitsEvent(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-orphans", UID: types.UID("node-orphans")}}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "orphan-0"},
		Status:     v1alpha1.GPUDeviceStatus{NodeName: node.Name},
	}
	base := newTestClient(t, scheme, node, device)
	rec, recorder := newTestRecorder(10)
	svc := NewCleanupService(base, recorder)

	if err := svc.RemoveOrphans(ctx, node, map[string]struct{}{device.Name: {}}); err != nil {
		t.Fatalf("RemoveOrphans returned error: %v", err)
	}

	if err := base.Get(ctx, types.NamespacedName{Name: device.Name}, &v1alpha1.GPUDevice{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected device to be deleted, got err=%v", err)
	}

	select {
	case event := <-rec.Events:
		if !strings.Contains(event, invstate.EventDeviceRemoved) {
			t.Fatalf("expected %q event, got %q", invstate.EventDeviceRemoved, event)
		}
	default:
		t.Fatalf("expected event to be recorded")
	}
}

func TestCleanupServiceRemoveOrphansDeleteError(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-orphans-error"}}
	base := newTestClient(t, scheme, node)

	boom := errors.New("delete failed")
	cl := &cleanupDelegatingClient{
		Client: base,
		delete: func(context.Context, client.Object, ...client.DeleteOption) error {
			return boom
		},
	}

	svc := NewCleanupService(cl, newTestRecorderLogger(1))
	if err := svc.RemoveOrphans(context.Background(), node, map[string]struct{}{"missing": {}}); !errors.Is(err, boom) {
		t.Fatalf("expected error %v, got %v", boom, err)
	}

	notFound := apierrors.NewNotFound(schema.GroupResource{Group: v1alpha1.GroupVersion.Group, Resource: "gpudevices"}, "missing")
	cl.delete = func(context.Context, client.Object, ...client.DeleteOption) error {
		return notFound
	}
	if err := svc.RemoveOrphans(context.Background(), node, map[string]struct{}{"missing": {}}); err != nil {
		t.Fatalf("expected notfound to be ignored, got %v", err)
	}
}
