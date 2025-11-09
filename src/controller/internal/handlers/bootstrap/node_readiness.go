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

package bootstrap

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// NodeReadinessHandler ensures that a GPU node has the basic readiness conditions set.
type NodeReadinessHandler struct {
	log logr.Logger
}

func NewNodeReadinessHandler(log logr.Logger) *NodeReadinessHandler {
	return &NodeReadinessHandler{log: log}
}

func (h *NodeReadinessHandler) Name() string {
	return "node-readiness"
}

func (h *NodeReadinessHandler) HandleNode(_ context.Context, inventory *v1alpha1.GPUNodeInventory) (contracts.Result, error) {
	ready := false
	for _, cond := range inventory.Status.Conditions {
		if cond.Type == conditionReadyForPooling && cond.Status == metav1.ConditionTrue {
			ready = true
			break
		}
	}

	if !ready {
		h.log.V(2).Info("node not ready for pooling", "node", inventory.Name)
		return contracts.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
	}

	return contracts.Result{}, nil
}
