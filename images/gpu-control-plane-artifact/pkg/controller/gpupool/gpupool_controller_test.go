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

package gpupool

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	testutil "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/internal/testutil"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

func TestSetupControllerBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("propagates-controller-add-error", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()

		addErr := errors.New("add fail")
		mgr := &testutil.StubManager{
			Cache:          &testutil.FakeCache{},
			Client:         cl,
			Scheme:         scheme,
			Recorder:       record.NewFakeRecorder(8),
			AddErr:         addErr,
			ControllerOpts: ctrlconfig.Controller{},
		}

		err := SetupController(ctx, mgr, testr.New(t), config.ControllerConfig{}, nil)
		if !errors.Is(err, addErr) {
			t.Fatalf("expected add error, got %v", err)
		}
	})

	t.Run("propagates-reconciler-setup-error", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()

		mgr := &testutil.StubManager{
			Cache:          nil,
			Client:         cl,
			Scheme:         scheme,
			Recorder:       record.NewFakeRecorder(8),
			ControllerOpts: ctrlconfig.Controller{},
		}

		err := SetupController(ctx, mgr, testr.New(t), config.ControllerConfig{}, nil)
		if err == nil {
			t.Fatalf("expected setup error")
		}
	})

	t.Run("propagates-gpupool-webhook-error", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme) // omit v1alpha1 to make GPUPool webhook registration fail
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()

		mgr := &testutil.StubManager{
			Cache:          &testutil.FakeCache{},
			Client:         cl,
			Scheme:         scheme,
			Recorder:       record.NewFakeRecorder(8),
			WebhookServer:  testutil.NewStubWebhookServer(),
			ControllerOpts: ctrlconfig.Controller{},
		}

		err := SetupController(ctx, mgr, testr.New(t), config.ControllerConfig{}, nil)
		if err == nil {
			t.Fatalf("expected webhook setup error")
		}
	})

	t.Run("propagates-pod-webhook-error", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = v1alpha1.AddToScheme(scheme) // omit corev1 to make Pod webhook registration fail
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()

		mgr := &testutil.StubManager{
			Cache:          &testutil.FakeCache{},
			Client:         cl,
			Scheme:         scheme,
			Recorder:       record.NewFakeRecorder(8),
			WebhookServer:  testutil.NewStubWebhookServer(),
			ControllerOpts: ctrlconfig.Controller{},
		}

		err := SetupController(ctx, mgr, testr.New(t), config.ControllerConfig{}, nil)
		if err == nil {
			t.Fatalf("expected webhook setup error")
		}
	})

	t.Run("success", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()

		state := moduleconfig.DefaultState()
		state.Settings.Placement.CustomTolerationKeys = []string{"example.com/key"}
		store := moduleconfig.NewModuleConfigStore(state)

		mgr := &testutil.StubManager{
			Cache:          &testutil.FakeCache{},
			Client:         cl,
			Scheme:         scheme,
			Recorder:       record.NewFakeRecorder(8),
			WebhookServer:  testutil.NewStubWebhookServer(),
			ControllerOpts: ctrlconfig.Controller{},
		}

		cfg := config.ControllerConfig{Workers: 0}
		if err := SetupController(ctx, mgr, testr.New(t), cfg, store); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
