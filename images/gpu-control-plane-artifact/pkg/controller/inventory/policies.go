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
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

func (r *Reconciler) currentPolicies() (invstate.ManagedNodesPolicy, invstate.DeviceApprovalPolicy) {
	if r.store != nil {
		state := r.store.Current()
		managed, approval, err := managedAndApprovalFromState(state)
		if err != nil {
			if r.log.GetSink() != nil {
				r.log.Error(err, "failed to build device approval policy from store, using fallback")
			}
		} else {
			return managed, approval
		}
	}

	return r.fallbackManaged, r.fallbackApproval
}

func (r *Reconciler) applyInventoryResync(state moduleconfig.State) {
	if state.Inventory.ResyncPeriod == "" {
		return
	}
	duration, err := time.ParseDuration(state.Inventory.ResyncPeriod)
	if err != nil || duration <= 0 {
		return
	}
	r.setResyncPeriod(duration)
}

func (r *Reconciler) refreshInventorySettings() {
	if r.store == nil {
		return
	}
	state := r.store.Current()
	r.applyInventoryResync(state)
}

func (r *Reconciler) setResyncPeriod(period time.Duration) {
	r.resyncMu.Lock()
	r.resyncPeriod = period
	r.resyncMu.Unlock()
}

func (r *Reconciler) requeueAllNodes(ctx context.Context) []reconcile.Request {
	nodeList := &corev1.NodeList{}
	if err := r.client.List(ctx, nodeList, client.MatchingLabels{"gpu.deckhouse.io/present": "true"}); err != nil {
		if r.log.GetSink() != nil {
			r.log.Error(err, "list GPU nodes to resync after module config change")
		}
		return nil
	}
	requests := make([]reconcile.Request, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		nodeName := nodeList.Items[i].Name
		if nodeName == "" {
			continue
		}
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}})
	}
	sort.Slice(requests, func(i, j int) bool {
		return requests[i].Name < requests[j].Name
	})
	return requests
}

func (r *Reconciler) mapModuleConfig(ctx context.Context, _ *unstructured.Unstructured) []reconcile.Request {
	if r.store != nil && !r.store.Current().Enabled {
		return nil
	}
	r.refreshInventorySettings()
	return r.requeueAllNodes(ctx)
}
