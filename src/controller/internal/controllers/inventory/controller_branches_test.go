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

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controllerbuilder"
)

func TestNodeFeatureSourceBuilderIsUsable(t *testing.T) {
	if src := nodeFeatureSourceBuilder(dummyCache{}); src == nil {
		t.Fatalf("expected source to be created")
	}
}

func TestSetupWithManagerDefaultsBuilderFactoryWhenNil(t *testing.T) {
	scheme := newTestScheme(t)
	indexer := &fakeFieldIndexer{}
	builder := &fakeControllerBuilder{}
	fakeSource := &fakeSyncingSource{}

	rec := &Reconciler{cfg: config.ControllerConfig{Workers: 1}}
	rec.nodeFeatureSourceFactory = func(cache.Cache) source.SyncingSource { return fakeSource }

	var defaultCalled bool
	orig := defaultBuilderFactory
	defaultBuilderFactory = func(ctrl.Manager) controllerbuilder.Builder {
		defaultCalled = true
		return builder
	}
	defer func() { defaultBuilderFactory = orig }()

	mgr := stubManager{
		client:   newTestClient(scheme),
		scheme:   scheme,
		recorder: record.NewFakeRecorder(1),
		indexer:  indexer,
		cache:    &fakeCache{},
	}

	if err := rec.SetupWithManager(context.Background(), mgr); err != nil {
		t.Fatalf("SetupWithManager returned error: %v", err)
	}
	if !defaultCalled {
		t.Fatalf("expected default builder factory to be used")
	}
	if !builder.completed {
		t.Fatalf("expected builder Complete to be called")
	}
}

type errorDetectionCollector struct{}

func (errorDetectionCollector) Collect(context.Context, string) (nodeDetection, error) {
	return nodeDetection{}, errors.New("boom")
}

type errorCleanupService struct {
	err error
}

func (e errorCleanupService) CleanupNode(context.Context, string) error     { return nil }
func (e errorCleanupService) DeleteInventory(context.Context, string) error { return nil }
func (e errorCleanupService) ClearMetrics(string)                           {}
func (e errorCleanupService) RemoveOrphans(context.Context, *corev1.Node, map[string]struct{}) error {
	return e.err
}

func TestReconcileRecordsDetectionUnavailableAndDeletionCleanup(t *testing.T) {
	// Telemetry unavailable branch.
	{
		scheme := newTestScheme(t)
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-telemetry",
				UID:  types.UID("node-worker-telemetry"),
				Labels: map[string]string{
					"gpu.deckhouse.io/device.00.vendor": "10de",
					"gpu.deckhouse.io/device.00.device": "1db5",
					"gpu.deckhouse.io/device.00.class":  "0302",
				},
			},
		}

		client := newTestClient(scheme, node)
		reconciler, err := New(testr.New(t), config.ControllerConfig{}, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error constructing reconciler: %v", err)
		}
		reconciler.client = client
		reconciler.scheme = scheme
		reconciler.recorder = record.NewFakeRecorder(16)
		reconciler.detectionCollector = errorDetectionCollector{}
		reconciler.detectionClient = reconciler.client

		if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
			t.Fatalf("expected reconcile to succeed when telemetry unavailable: %v", err)
		}
	}

	// Deletion cleanup branch.
	{
		scheme := newTestScheme(t)
		now := metav1.Now()
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "worker-deleting",
				UID:               types.UID("node-worker-deleting"),
				DeletionTimestamp: &now,
				Finalizers:        []string{"test.finalizer"},
				Labels: map[string]string{
					"gpu.deckhouse.io/device.00.vendor": "10de",
					"gpu.deckhouse.io/device.00.device": "1db5",
					"gpu.deckhouse.io/device.00.class":  "0302",
				},
			},
		}
		client := newTestClient(scheme, node)
		reconciler, err := New(testr.New(t), config.ControllerConfig{}, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error constructing reconciler: %v", err)
		}
		reconciler.client = client
		reconciler.scheme = scheme
		reconciler.recorder = record.NewFakeRecorder(16)
		reconciler.cleanupService = errorCleanupService{err: errors.New("cleanup boom")}

		if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err == nil {
			t.Fatalf("expected reconcile to return cleanup error")
		}
	}
}
