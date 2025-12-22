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

package state

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestInventoryStateAllowCleanup(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}

	state := NewInventoryState(node, nil, ManagedNodesPolicy{}, DeviceApprovalPolicy{})
	if state.AllowCleanup() {
		t.Fatalf("expected cleanup to be disabled without devices and features")
	}

	state = NewInventoryState(node, &nfdv1alpha1.NodeFeature{}, ManagedNodesPolicy{}, DeviceApprovalPolicy{})
	if !state.AllowCleanup() {
		t.Fatalf("expected cleanup to be enabled when NodeFeature detected")
	}

	node.Labels = map[string]string{
		"gpu.deckhouse.io/device.00.vendor": "10de",
		"gpu.deckhouse.io/device.00.device": "1db5",
		"gpu.deckhouse.io/device.00.class":  "0302",
	}
	state = NewInventoryState(node, nil, ManagedNodesPolicy{}, DeviceApprovalPolicy{})
	if !state.AllowCleanup() {
		t.Fatalf("expected cleanup to be enabled when devices are present")
	}
}

func TestInventoryStateOrphanDevicesListsExistingGPUDevices(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}
	state := NewInventoryState(node, nil, ManagedNodesPolicy{}, DeviceApprovalPolicy{})

	c := &delegatingClient{
		list: func(_ context.Context, list client.ObjectList, opts ...client.ListOption) error {
			listOpts := &client.ListOptions{}
			for _, opt := range opts {
				opt.ApplyToList(listOpts)
			}

			want := fields.SelectorFromSet(fields.Set{DeviceNodeIndexKey: "node-a"}).String()
			if listOpts.FieldSelector == nil || listOpts.FieldSelector.String() != want {
				return errors.New("unexpected field selector")
			}

			devices, ok := list.(*v1alpha1.GPUDeviceList)
			if !ok {
				return errors.New("unexpected list type")
			}
			devices.Items = []v1alpha1.GPUDevice{
				{ObjectMeta: metav1.ObjectMeta{Name: "dev-a"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "dev-b"}},
			}
			return nil
		},
	}

	orphanDevices, err := state.OrphanDevices(context.Background(), c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(orphanDevices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(orphanDevices))
	}
	if _, ok := orphanDevices["dev-a"]; !ok {
		t.Fatalf("expected dev-a to be present")
	}
	if _, ok := orphanDevices["dev-b"]; !ok {
		t.Fatalf("expected dev-b to be present")
	}
}
