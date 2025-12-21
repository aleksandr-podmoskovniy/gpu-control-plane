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

package poolusage

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	puinternal "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/poolusage/internal"
)

type ClusterGPUPoolUsageReconciler struct {
	client   client.Client
	log      logr.Logger
	cfg      config.ControllerConfig
	store    *moduleconfig.ModuleConfigStore
	handlers []puinternal.ClusterGPUPoolHandler
}

func NewClusterGPUPoolUsage(log logr.Logger, cfg config.ControllerConfig, store *moduleconfig.ModuleConfigStore) *ClusterGPUPoolUsageReconciler {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	return &ClusterGPUPoolUsageReconciler{
		log:   log,
		cfg:   cfg,
		store: store,
		handlers: []puinternal.ClusterGPUPoolHandler{
			puinternal.NewClusterGPUPoolUsageHandler(),
		},
	}
}

var _ reconcile.Reconciler = (*ClusterGPUPoolUsageReconciler)(nil)
