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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestDeviceApprovalAutoAttachPolicies(t *testing.T) {
	baseModule := defaultModuleSettings()

	tests := []struct {
		name           string
		configure      func(*config.ModuleSettings)
		wantAutoAttach bool
	}{
		{
			name: "manual",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeManual
				m.DeviceApproval.Selector = nil
			},
			wantAutoAttach: false,
		},
		{
			name: "automatic",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeAutomatic
				m.DeviceApproval.Selector = nil
			},
			wantAutoAttach: true,
		},
		{
			name: "selector-match",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeSelector
				m.DeviceApproval.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"gpu.deckhouse.io/device.vendor": "10de"},
				}
			},
			wantAutoAttach: true,
		},
		{
			name: "selector-miss",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeSelector
				m.DeviceApproval.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"gpu.deckhouse.io/device.vendor": "1234"},
				}
			},
			wantAutoAttach: false,
		},
		{
			name: "selector-empty",
			configure: func(m *config.ModuleSettings) {
				m.DeviceApproval.Mode = config.DeviceApprovalModeSelector
				m.DeviceApproval.Selector = nil
			},
			wantAutoAttach: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			module := baseModule
			tt.configure(&module)

			scheme := newTestScheme(t)
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "auto-node-" + tt.name,
					UID:  types.UID("node-" + tt.name),
					Labels: map[string]string{
						"gpu.deckhouse.io/device.00.vendor": "10de",
						"gpu.deckhouse.io/device.00.device": "1db5",
						"gpu.deckhouse.io/device.00.class":  "0302",
					},
				},
			}

			client := newTestClient(scheme, node)
			reconciler, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
			if err != nil {
				t.Fatalf("unexpected error constructing reconciler: %v", err)
			}
			reconciler.client = client
			reconciler.scheme = scheme
			reconciler.recorder = record.NewFakeRecorder(32)

			ctx := context.Background()
			if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: node.Name}}); err != nil {
				t.Fatalf("unexpected reconcile error: %v", err)
			}

			deviceName := buildDeviceName(node.Name, deviceSnapshot{Index: "0", Vendor: "10de", Device: "1db5", Class: "0302"})
			device := &v1alpha1.GPUDevice{}
			if err := client.Get(ctx, types.NamespacedName{Name: deviceName}, device); err != nil {
				t.Fatalf("expected device, got error: %v", err)
			}
			if device.Status.AutoAttach != tt.wantAutoAttach {
				t.Fatalf("autoAttach mismatch: want %v got %v", tt.wantAutoAttach, device.Status.AutoAttach)
			}
		})
	}
}

func TestDeviceApprovalAutoAttachRespectsManagedFlag(t *testing.T) {
	policy := DeviceApprovalPolicy{Mode: moduleconfig.DeviceApprovalModeAutomatic}
	if policy.AutoAttach(false, labels.Set{}) {
		t.Fatalf("auto attach should be false when node not managed")
	}
}
