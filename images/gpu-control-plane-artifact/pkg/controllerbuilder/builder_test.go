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

package controllerbuilder

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type noopSource struct{}

func (noopSource) Start(context.Context, workqueue.RateLimitingInterface) error { return nil }

func TestRuntimeBuilderChainsCallsAndFailsCompleteWithoutManager(t *testing.T) {
	b := NewManagedBy(nil)
	if b == nil {
		t.Fatalf("expected builder instance")
	}

	b = b.Named("test").
		For(&corev1.Pod{}, builder.WithPredicates()).
		Owns(&appsv1.Deployment{}).
		WatchesRawSource(noopSource{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1})

	err := b.Complete(reconcile.Func(func(context.Context, reconcile.Request) (reconcile.Result, error) {
		return reconcile.Result{}, nil
	}))
	if err == nil {
		t.Fatalf("expected Complete to fail without Manager")
	}

	// Cover type assertion that production controllers rely on for injections.
	if _, ok := any(b).(*runtimeBuilder); !ok {
		t.Fatalf("expected runtime builder implementation, got %T", b)
	}

	// Ensure we satisfy the controller-runtime Source interface.
	var _ source.Source = noopSource{}
}
