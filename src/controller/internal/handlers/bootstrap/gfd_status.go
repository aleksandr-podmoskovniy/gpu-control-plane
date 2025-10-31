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

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

const (
	conditionGFDReady = "GFDReady"
)

// GFDStatusHandler verifies that GPU Feature Discovery has reported data recently.
type GFDStatusHandler struct {
	log logr.Logger
}

func NewGFDStatusHandler(log logr.Logger) *GFDStatusHandler {
	return &GFDStatusHandler{log: log}
}

func (h *GFDStatusHandler) Name() string {
	return "gfd-status"
}

func (h *GFDStatusHandler) HandleNode(_ context.Context, inventory *v1alpha1.GPUNodeInventory) (contracts.Result, error) {
	for _, cond := range inventory.Status.Conditions {
		if cond.Type == conditionGFDReady && cond.Status == "True" {
			return contracts.Result{}, nil
		}
	}

	h.log.V(1).Info("waiting for GFD readiness", "node", inventory.Name)
	return contracts.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
}
