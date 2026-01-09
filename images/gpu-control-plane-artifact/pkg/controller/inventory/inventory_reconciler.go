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
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	invhandler "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/handler"
	invservice "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/service"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/eventrecord"
)

const (
	ControllerName = "gpu-inventory-controller"

	defaultResyncPeriod time.Duration = 0
)

type Handler interface {
	Handle(ctx context.Context, state invstate.InventoryState) (reconcile.Result, error)
	Name() string
}

type Reconciler struct {
	client           client.Client
	scheme           *runtime.Scheme
	log              logr.Logger
	cfg              config.ControllerConfig
	handlers         []Handler
	deviceHandlers   []invservice.DeviceHandler
	recorder         eventrecord.EventRecorderLogger
	resyncPeriod     time.Duration
	resyncMu         sync.RWMutex
	store            *moduleconfig.ModuleConfigStore
	fallbackManaged  invstate.ManagedNodesPolicy
	fallbackApproval invstate.DeviceApprovalPolicy

	detectionCollector invhandler.DetectionCollector
	cleanupService     invhandler.CleanupService
	deviceService      invhandler.DeviceService
	inventoryService   invhandler.InventoryService
	detectionClient    client.Client
}

func New(log logr.Logger, cfg config.ControllerConfig, store *moduleconfig.ModuleConfigStore, handlers []invservice.DeviceHandler) (*Reconciler, error) {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.ResyncPeriod <= 0 {
		cfg.ResyncPeriod = defaultResyncPeriod
	}

	state := moduleconfig.DefaultState()
	if store != nil {
		state = store.Current()
	}

	managed, approval, err := managedAndApprovalFromState(state)
	if err != nil {
		return nil, err
	}

	rec := &Reconciler{
		log:              log,
		cfg:              cfg,
		deviceHandlers:   handlers,
		store:            store,
		fallbackManaged:  managed,
		fallbackApproval: approval,
	}
	rec.setResyncPeriod(cfg.ResyncPeriod)
	rec.applyInventoryResync(state)

	return rec, nil
}

func NewReconciler(log logr.Logger, cfg config.ControllerConfig, store *moduleconfig.ModuleConfigStore, handlers []invservice.DeviceHandler) (*Reconciler, error) {
	return New(log, cfg, store, handlers)
}

func (r *Reconciler) detectionSvc() invhandler.DetectionCollector {
	if r.detectionCollector == nil || r.detectionClient != r.client {
		r.detectionCollector = invservice.NewDetectionCollector(r.client)
		r.detectionClient = r.client
	}
	return r.detectionCollector
}

func (r *Reconciler) cleanupSvc() invhandler.CleanupService {
	if r.cleanupService == nil {
		r.cleanupService = invservice.NewCleanupService(r.client, r.recorder)
	}
	return r.cleanupService
}

func (r *Reconciler) deviceSvc() invhandler.DeviceService {
	if r.deviceService == nil {
		r.deviceService = invservice.NewDeviceService(r.client, r.scheme, r.recorder, r.deviceHandlers)
	}
	return r.deviceService
}

func (r *Reconciler) inventorySvc() invhandler.InventoryService {
	if r.inventoryService == nil {
		r.inventoryService = invservice.NewInventoryService(r.client, r.scheme, r.recorder)
	}
	return r.inventoryService
}

func (r *Reconciler) handlerChain() []Handler {
	if r.handlers != nil {
		return r.handlers
	}
	r.handlers = []Handler{
		invhandler.NewInventoryHandler(
			r.log.WithName("inventory"),
			r.client,
			r.deviceSvc(),
			r.inventorySvc(),
			r.cleanupSvc(),
			r.detectionSvc(),
			r.recorder,
		),
	}
	return r.handlers
}

func managedAndApprovalFromState(state moduleconfig.State) (invstate.ManagedNodesPolicy, invstate.DeviceApprovalPolicy, error) {
	managed := invstate.ManagedNodesPolicy{
		LabelKey:         strings.TrimSpace(state.Settings.ManagedNodes.LabelKey),
		EnabledByDefault: state.Settings.ManagedNodes.EnabledByDefault,
	}
	if managed.LabelKey == "" {
		managed.LabelKey = invstate.DefaultManagedNodeLabelKey
	}

	approval, err := invstate.NewDeviceApprovalPolicy(state.Settings.DeviceApproval)
	if err != nil {
		return invstate.ManagedNodesPolicy{}, invstate.DeviceApprovalPolicy{}, err
	}

	return managed, approval, nil
}

var _ reconcile.Reconciler = (*Reconciler)(nil)
