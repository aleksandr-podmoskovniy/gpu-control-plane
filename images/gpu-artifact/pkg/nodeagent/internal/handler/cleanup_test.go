/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handler

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

func TestCleanupHandlerDeletesStaleObjects(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gpuv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	expectedName := state.PhysicalGPUName("node-1", "0000:01:00.0")
	staleName := state.PhysicalGPUName("node-1", "0000:02:00.0")

	expected := &gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{
			Name:   expectedName,
			Labels: map[string]string{state.LabelNode: "node-1"},
		},
	}
	stale := &gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{
			Name:   staleName,
			Labels: map[string]string{state.LabelNode: "node-1"},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(expected, stale).
		Build()

	handler := NewCleanupHandler(service.NewClientStore(cl))
	st := state.New("node-1")
	st.SetDevices([]state.Device{
		{Address: "0000:01:00.0"},
	})

	if err := handler.Handle(context.Background(), st); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if err := cl.Get(context.Background(), client.ObjectKey{Name: expectedName}, &gpuv1alpha1.PhysicalGPU{}); err != nil {
		t.Fatalf("expected object missing: %v", err)
	}

	if err := cl.Get(context.Background(), client.ObjectKey{Name: staleName}, &gpuv1alpha1.PhysicalGPU{}); err == nil {
		t.Fatalf("stale object still exists")
	} else if !apierrors.IsNotFound(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}
