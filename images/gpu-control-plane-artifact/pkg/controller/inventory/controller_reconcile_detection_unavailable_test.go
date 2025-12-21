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
	"strings"
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	invconsts "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/consts"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestReconcileRecordsDetectionUnavailableWhenCollectorFails(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-detect-fail",
			UID:  types.UID("worker-detect-fail"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "2203",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}

	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	baseClient := newTestClient(scheme, node)
	rec.client = baseClient
	rec.scheme = scheme
	recorder := record.NewFakeRecorder(8)
	rec.recorder = recorder
	rec.detectionCollector = failingDetectionsCollector{err: errors.New("boom")}
	rec.detectionClient = rec.client

	res, err := rec.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != (ctrl.Result{}) {
		t.Fatalf("expected empty result, got %+v", res)
	}

	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, invconsts.EventDetectUnavailable) {
			t.Fatalf("expected %q event, got %q", invconsts.EventDetectUnavailable, event)
		}
	default:
		t.Fatalf("expected detection-unavailable event")
	}
}
