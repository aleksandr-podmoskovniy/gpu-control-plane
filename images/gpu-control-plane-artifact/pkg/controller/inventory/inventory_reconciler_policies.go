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
	"time"

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

func (r *Reconciler) setResyncPeriod(period time.Duration) {
	r.resyncMu.Lock()
	r.resyncPeriod = period
	r.resyncMu.Unlock()
}
