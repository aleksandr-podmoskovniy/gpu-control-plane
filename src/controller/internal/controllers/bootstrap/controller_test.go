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

package bootstrap

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/go-logr/logr/testr"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

type stubBootstrapHandler struct {
	called bool
	result contracts.Result
}

func (s *stubBootstrapHandler) Name() string { return "stub" }

func (s *stubBootstrapHandler) HandleNode(context.Context, *gpuv1alpha1.GPUNodeInventory) (contracts.Result, error) {
	s.called = true
	return s.result, nil
}

func TestBootstrapReconcileAggregates(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = gpuv1alpha1.AddToScheme(scheme)

	node := &gpuv1alpha1.GPUNodeInventory{ObjectMeta: metav1.ObjectMeta{Name: "node"}}
	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()

	handler := &stubBootstrapHandler{result: contracts.Result{Requeue: true, RequeueAfter: time.Second}}
	reconciler := New(testr.New(t), config.ControllerConfig{}, []contracts.BootstrapHandler{handler})
	reconciler.client = client
	reconciler.scheme = scheme

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "node"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handler.called {
		t.Fatal("handler not executed")
	}
	if !res.Requeue || res.RequeueAfter != time.Second {
		t.Fatalf("unexpected result: %+v", res)
	}
}
