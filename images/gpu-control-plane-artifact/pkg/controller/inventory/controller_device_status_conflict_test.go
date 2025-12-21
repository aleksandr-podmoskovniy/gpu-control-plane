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

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

func TestCreateDeviceStatusConflictTriggersRequeue(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-conflict-create",
			UID:  types.UID("node-conflict-create"),
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
				"gpu.deckhouse.io/device.00.device": "1db5",
				"gpu.deckhouse.io/device.00.class":  "0302",
			},
		},
	}
	baseClient := newTestClient(scheme, node)
	statusWriter := &conflictStatusUpdater{StatusWriter: baseClient.Status()}
	client := &delegatingClient{Client: baseClient, statusWriter: statusWriter}

	reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(defaultModuleSettings()), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	reconciler.client = client
	reconciler.scheme = scheme
	reconciler.recorder = record.NewFakeRecorder(32)

	snapshot := deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"}
	device, result, err := reconciler.deviceSvc().Reconcile(context.Background(), node, snapshot, map[string]string{}, true, reconciler.fallbackApproval, nil)
	if err != nil {
		t.Fatalf("unexpected reconcileDevice error: %v", err)
	}
	if device == nil {
		t.Fatalf("expected device to be returned")
	}
	if !result.Requeue {
		t.Fatalf("expected requeue result when status update conflicts")
	}
}
