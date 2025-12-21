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
	"time"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestRequeueAllNodes(t *testing.T) {
	scheme := newTestScheme(t)
	nodeA := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{"gpu.deckhouse.io/present": "true"}}}
	nodeB := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-b", Labels: map[string]string{"gpu.deckhouse.io/present": "true"}}}
	nodeC := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-c", Labels: map[string]string{"gpu.deckhouse.io/present": "false"}}}
	reconciler := &Reconciler{client: newTestClient(scheme, nodeA, nodeB, nodeC)}

	reqs := reconciler.requeueAllNodes(context.Background())
	if len(reqs) != 2 {
		t.Fatalf("expected two requests, got %#v", reqs)
	}
	expected := map[string]struct{}{"node-a": {}, "node-b": {}}
	for _, req := range reqs {
		if _, ok := expected[req.Name]; !ok {
			t.Fatalf("unexpected request %#v", req)
		}
	}

	nodeNoLabel := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-no-label"}}
	reconciler = &Reconciler{client: newTestClient(scheme, nodeA, nodeB, nodeC, nodeNoLabel)}
	if reqs := reconciler.requeueAllNodes(context.Background()); len(reqs) != 2 {
		t.Fatalf("expected nodes without present=true to be skipped, got %v", reqs)
	}
}

func TestRequeueAllNodesHandlesError(t *testing.T) {
	reconciler := &Reconciler{
		client: &failingListClient{err: errors.New("list fail")},
		log:    testr.New(t),
	}

	if reqs := reconciler.requeueAllNodes(context.Background()); len(reqs) != 0 {
		t.Fatalf("expected empty result on error, got %#v", reqs)
	}
}

func TestRequeueAllNodesEmptyList(t *testing.T) {
	scheme := newTestScheme(t)
	reconciler := &Reconciler{client: newTestClient(scheme)}
	reqs := reconciler.requeueAllNodes(context.Background())
	if len(reqs) != 0 {
		t.Fatalf("expected no requests for empty node list, got %v", reqs)
	}

	reconciler.log = testr.New(t)
	reconciler.client = &failingListClient{err: errors.New("list err")}
	if reqs := reconciler.requeueAllNodes(context.Background()); len(reqs) != 0 {
		t.Fatalf("expected error branch to return empty slice, got %v", reqs)
	}
}

func TestRequeueAllNodesSkipsEmptyNodeNames(t *testing.T) {
	reconciler := &Reconciler{
		client: &delegatingClient{
			Client: newTestClient(newTestScheme(t)),
			list: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				nodeList, ok := list.(*corev1.NodeList)
				if !ok {
					return nil
				}
				nodeList.Items = []corev1.Node{
					{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}},
					{ObjectMeta: metav1.ObjectMeta{Name: ""}},
				}
				return nil
			},
		},
	}

	reqs := reconciler.requeueAllNodes(context.Background())
	if len(reqs) != 1 || reqs[0].Name != "node-a" {
		t.Fatalf("unexpected requests: %#v", reqs)
	}
}

func TestMapModuleConfigRequeuesNodes(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-map", Labels: map[string]string{"gpu.deckhouse.io/present": "true"}}}
	reconciler := &Reconciler{
		client: newTestClient(scheme, node),
	}

	reqs := reconciler.mapModuleConfig(context.Background(), nil)
	if len(reqs) != 1 || reqs[0].Name != node.Name {
		t.Fatalf("unexpected requests returned from mapModuleConfig: %#v", reqs)
	}
}

func TestMapModuleConfigSkipsNodesWithoutDevices(t *testing.T) {
	scheme := newTestScheme(t)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-empty"}}
	reconciler := &Reconciler{
		client: newTestClient(scheme, node),
	}

	if reqs := reconciler.mapModuleConfig(context.Background(), nil); len(reqs) != 0 {
		t.Fatalf("expected no requests when there are no GPU devices, got %#v", reqs)
	}
}

func TestMapModuleConfigCleansResourcesWhenDisabled(t *testing.T) {
	scheme := newTestScheme(t)
	inv := &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	dev := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: "dev1"}, Status: v1alpha1.GPUDeviceStatus{NodeName: "node1"}}
	client := newTestClient(scheme, inv, dev)

	store := moduleconfig.NewModuleConfigStore(moduleconfig.State{Enabled: false, Settings: moduleconfig.DefaultState().Settings})

	rec := &Reconciler{
		client:          client,
		store:           store,
		fallbackManaged: ManagedNodesPolicy{LabelKey: "gpu.deckhouse.io/enabled", EnabledByDefault: true},
	}
	if reqs := rec.mapModuleConfig(context.Background(), nil); len(reqs) != 0 {
		t.Fatalf("expected no requeues when module disabled")
	}
	// cleanup on disable handled by pre-delete hook; controller should not delete CRs here
	if err := client.Get(context.Background(), types.NamespacedName{Name: "node1"}, &v1alpha1.GPUNodeState{}); err != nil {
		t.Fatalf("inventory should remain, got %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "dev1"}, &v1alpha1.GPUDevice{}); err != nil {
		t.Fatalf("device should remain, got %v", err)
	}
}

func TestMapModuleConfigCleanupErrorIsIgnored(t *testing.T) {
	rec := &Reconciler{
		client: &listErrorClient{err: errors.New("cleanup fail")},
		log:    testr.New(t),
		store:  moduleconfig.NewModuleConfigStore(moduleconfig.State{Enabled: false, Settings: moduleconfig.DefaultState().Settings}),
	}
	if reqs := rec.mapModuleConfig(context.Background(), nil); reqs != nil {
		t.Fatalf("expected nil requests when cleanup fails, got %#v", reqs)
	}
}

func TestApplyInventoryResyncUpdatesPeriod(t *testing.T) {
	reconciler := &Reconciler{}
	reconciler.setResyncPeriod(2 * time.Minute)

	state := moduleconfig.DefaultState()
	state.Inventory.ResyncPeriod = "15s"

	reconciler.applyInventoryResync(state)

	if got := reconciler.resyncPeriod; got != 15*time.Second {
		t.Fatalf("expected resync period 15s, got %s", got)
	}
}

func TestApplyInventoryResyncIgnoresInvalidValue(t *testing.T) {
	reconciler := &Reconciler{}
	reconciler.setResyncPeriod(45 * time.Second)

	state := moduleconfig.DefaultState()
	state.Inventory.ResyncPeriod = "not-a-duration"

	reconciler.applyInventoryResync(state)

	if got := reconciler.resyncPeriod; got != 45*time.Second {
		t.Fatalf("resync period should remain unchanged, got %s", got)
	}
}

func TestApplyInventoryResyncIgnoresEmptyValue(t *testing.T) {
	reconciler := &Reconciler{}
	reconciler.setResyncPeriod(time.Minute)

	state := moduleconfig.DefaultState()
	state.Inventory.ResyncPeriod = ""

	reconciler.applyInventoryResync(state)

	if got := reconciler.resyncPeriod; got != time.Minute {
		t.Fatalf("resync period should remain unchanged when value empty, got %s", got)
	}
}

func TestRefreshInventorySettingsReadsStore(t *testing.T) {
	state := moduleconfig.DefaultState()
	state.Inventory.ResyncPeriod = "90s"
	store := moduleconfig.NewModuleConfigStore(state)

	reconciler := &Reconciler{store: store}
	reconciler.setResyncPeriod(30 * time.Second)

	reconciler.refreshInventorySettings()

	if got := reconciler.resyncPeriod; got != 90*time.Second {
		t.Fatalf("expected resync period from store (90s), got %s", got)
	}
}
