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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

func TestDeviceServiceReconcileAutoAttachPolicies(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)
	snapshot := newTestSnapshot()

	tests := []struct {
		name       string
		settings   moduleconfig.DeviceApprovalSettings
		managed    bool
		autoAttach bool
	}{
		{
			name:       "manual",
			settings:   moduleconfig.DeviceApprovalSettings{Mode: moduleconfig.DeviceApprovalModeManual},
			managed:    true,
			autoAttach: false,
		},
		{
			name:       "automatic",
			settings:   moduleconfig.DeviceApprovalSettings{Mode: moduleconfig.DeviceApprovalModeAutomatic},
			managed:    true,
			autoAttach: true,
		},
		{
			name: "selector-match",
			settings: moduleconfig.DeviceApprovalSettings{
				Mode: moduleconfig.DeviceApprovalModeSelector,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"gpu.deckhouse.io/device.vendor": "10de"},
				},
			},
			managed:    true,
			autoAttach: true,
		},
		{
			name: "selector-miss",
			settings: moduleconfig.DeviceApprovalSettings{
				Mode: moduleconfig.DeviceApprovalModeSelector,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"gpu.deckhouse.io/device.vendor": "1234"},
				},
			},
			managed:    true,
			autoAttach: false,
		},
		{
			name:       "selector-empty",
			settings:   moduleconfig.DeviceApprovalSettings{Mode: moduleconfig.DeviceApprovalModeSelector},
			managed:    true,
			autoAttach: true,
		},
		{
			name:       "unmanaged",
			settings:   moduleconfig.DeviceApprovalSettings{Mode: moduleconfig.DeviceApprovalModeAutomatic},
			managed:    false,
			autoAttach: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			node := newTestNode("node-autoattach-" + tt.name)
			base := newTestClient(t, scheme, node)

			policy, err := invstate.NewDeviceApprovalPolicy(tt.settings)
			if err != nil {
				t.Fatalf("unexpected policy error: %v", err)
			}

			svc := NewDeviceService(base, scheme, nil, nil)
			device, _, err := svc.Reconcile(ctx, node, snapshot, map[string]string{}, tt.managed, policy, nil)
			if err != nil {
				t.Fatalf("Reconcile returned error: %v", err)
			}
			if device.Status.AutoAttach != tt.autoAttach {
				t.Fatalf("autoAttach mismatch: want %v got %v", tt.autoAttach, device.Status.AutoAttach)
			}
		})
	}
}
