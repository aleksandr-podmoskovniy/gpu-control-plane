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

package handlers

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/handlers/gpupool"
	moduleconfig "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
)

func TestRegisterDefaultsWithClient(t *testing.T) {
	scheme := fake.NewClientBuilder().Build().Scheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	deps := &Handlers{Client: cl}
	RegisterDefaults(testr.New(t), deps)

	if len(deps.Pool.List()) == 0 {
		t.Fatalf("expected pool handlers registered")
	}
	if len(deps.Admission.List()) == 0 {
		t.Fatalf("expected admission handlers registered")
	}
}

func TestRegisterDefaultsUsesModuleConfigStore(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	state := moduleconfig.DefaultState()
	state.Settings.Placement.CustomTolerationKeys = []string{"tol-a", "tol-b"}
	store := config.NewModuleConfigStore(state)

	t.Setenv("NVIDIA_DEVICE_PLUGIN_IMAGE", "img")
	t.Setenv("POD_NAMESPACE", "ns")

	deps := &Handlers{Client: cl, ModuleConfigStore: store}
	RegisterDefaults(testr.New(t), deps)

	var renderer *gpupool.RendererHandler
	for _, h := range deps.Pool.List() {
		if r, ok := h.(*gpupool.RendererHandler); ok {
			renderer = r
			break
		}
	}
	if renderer == nil {
		t.Fatalf("renderer handler must be registered when client present")
	}

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec: v1alpha1.GPUPoolSpec{
			Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
			Scheduling: v1alpha1.GPUPoolSchedulingSpec{
				TaintsEnabled: ptr.To(true),
			},
		},
		Status: v1alpha1.GPUPoolStatus{
			Capacity: v1alpha1.GPUPoolCapacityStatus{
				Total: 1,
			},
		},
	}
	if _, err := renderer.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("renderer HandlePool: %v", err)
	}
	ds := &appsv1.DaemonSet{}
	if err := cl.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "nvidia-device-plugin-pool"}, ds); err != nil {
		t.Fatalf("expected daemonset created, got %v", err)
	}
	tolerations := ds.Spec.Template.Spec.Tolerations
	foundCustom := false
	for _, tol := range tolerations {
		if tol.Key == "tol-a" || tol.Key == "tol-b" {
			foundCustom = true
		}
	}
	if !foundCustom {
		t.Fatalf("expected custom tolerations to be applied, got %v", tolerations)
	}
}
