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

package admission

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// PoolSnapshotHandler serialises pool status for admission webhook consumption.
type PoolSnapshotHandler struct {
	log logr.Logger
}

func NewPoolSnapshotHandler(log logr.Logger) *PoolSnapshotHandler {
	return &PoolSnapshotHandler{log: log}
}

func (h *PoolSnapshotHandler) Name() string {
	return "pool-snapshot"
}

func (h *PoolSnapshotHandler) SyncPool(_ context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	payload, err := json.Marshal(pool.Status)
	if err != nil {
		return contracts.Result{}, err
	}

	if pool.Annotations == nil {
		pool.Annotations = make(map[string]string)
	}
	pool.Annotations["gpu.deckhouse.io/pool-status"] = string(payload)
	return contracts.Result{}, nil
}
