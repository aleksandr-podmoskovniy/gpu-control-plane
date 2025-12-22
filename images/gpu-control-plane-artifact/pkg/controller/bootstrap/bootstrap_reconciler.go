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
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	common "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	bshandler "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap/internal/watcher"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
	bootmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics/bootstrap"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/validation"
)

const (
	ControllerName             = "gpu-bootstrap-controller"
	cacheSyncTimeoutDuration   = 10 * time.Minute
	conditionInventoryComplete = "InventoryComplete"
	conditionDriverReady       = "DriverReady"
	conditionToolkitReady      = "ToolkitReady"
	conditionMonitoringReady   = "MonitoringReady"
	conditionReadyForPooling   = "ReadyForPooling"
	conditionWorkloadsDegraded = "WorkloadsDegraded"
)

var (
	bootstrapPhases = []string{
		"Validating",
		"Monitoring",
		"Ready",
	}
	bootstrapConditionTypes = []string{
		conditionInventoryComplete,
		conditionDriverReady,
		conditionToolkitReady,
		conditionMonitoringReady,
		conditionReadyForPooling,
		conditionWorkloadsDegraded,
	}
)

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Handler = bshandler.Handler

type Reconciler struct {
	client    client.Client
	log       logr.Logger
	cfg       config.ControllerConfig
	store     *moduleconfig.ModuleConfigStore
	handlers  []Handler
	validator validation.Validator
}

func New(log logr.Logger, cfg config.ControllerConfig, store *moduleconfig.ModuleConfigStore, handlers []Handler) *Reconciler {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	rec := &Reconciler{
		log:      log,
		cfg:      cfg,
		store:    store,
		handlers: handlers,
	}
	return rec
}

func (r *Reconciler) SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error {
	r.client = mgr.GetClient()
	r.injectClient()

	if fieldIndexer := mgr.GetFieldIndexer(); fieldIndexer != nil {
		if err := fieldIndexer.IndexField(ctx, &corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			if pod, ok := obj.(*corev1.Pod); ok && pod.Spec.NodeName != "" {
				return []string{pod.Spec.NodeName}
			}
			return nil
		}); err != nil {
			return err
		}
	}

	c := mgr.GetCache()
	if c == nil {
		return fmt.Errorf("manager cache is required")
	}

	if err := ctr.Watch(
		source.Kind(c, &v1alpha1.GPUNodeState{}, &handler.TypedEnqueueRequestForObject[*v1alpha1.GPUNodeState]{}),
	); err != nil {
		return fmt.Errorf("error setting watch on GPUNodeState: %w", err)
	}

	for _, w := range []Watcher{
		watcher.NewWorkloadPodWatcher(r.log.WithName("watcher.workloadPod")),
		watcher.NewGPUDeviceWatcher(r.log.WithName("watcher.device")),
	} {
		if err := w.Watch(mgr, ctr); err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) injectClient() {
	if r.client == nil {
		return
	}
	if r.validator == nil {
		r.validator = validation.NewValidator(r.client, r.validatorConfig())
	}
	for _, handler := range r.handlers {
		if setter, ok := handler.(interface{ SetClient(client.Client) }); ok {
			setter.SetClient(r.client)
		}
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := crlog.FromContext(ctx).WithValues("inventory", req.Name)
	ctx = logr.NewContext(ctx, log)

	resource := reconciler.NewResource(
		types.NamespacedName{Name: req.Name},
		r.client,
		func() *v1alpha1.GPUNodeState { return &v1alpha1.GPUNodeState{} },
		func(obj *v1alpha1.GPUNodeState) v1alpha1.GPUNodeStateStatus { return obj.Status },
	)
	if err := resource.Fetch(ctx); err != nil {
		return ctrl.Result{}, err
	}
	if resource.IsEmpty() {
		log.V(2).Info("GPUNodeState removed")
		r.clearBootstrapMetrics(req.Name)
		return ctrl.Result{}, nil
	}

	inventory := resource.Changed()

	if r.store != nil && !r.store.Current().Enabled {
		log.V(1).Info("module disabled, skipping bootstrap reconciliation")
		r.clearBootstrapMetrics(req.Name)
		return ctrl.Result{}, nil
	}

	if r.validator == nil {
		r.validator = validation.NewValidator(r.client, r.validatorConfig())
	}
	prevPhase := effectiveBootstrapPhase(resource.Current())
	s := state.New(r.client, inventory)

	// Capture validator status and pass via context for handlers if needed.
	nodeName := inventory.Spec.NodeName
	if nodeName == "" {
		nodeName = inventory.Name
	}
	if status, err := r.validator.Status(ctx, nodeName); err == nil {
		ctx = validation.ContextWithStatus(ctx, status)
	} else {
		log.Error(err, "validator status failed")
	}

	rec := reconciler.NewBaseReconciler(r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, handler Handler) (reconcile.Result, error) {
		result, err := handler.Handle(ctx, s)
		if err != nil {
			bootmetrics.BootstrapHandlerErrorInc(handler.Name())
		}
		return result, err
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		if equality.Semantic.DeepEqual(resource.Current().Status, inventory.Status) {
			return nil
		}
		return resource.Update(ctx)
	})

	res, err := rec.Reconcile(ctx)
	if err != nil {
		log.Error(err, "handler chain failed")
		return ctrl.Result{}, err
	}
	r.updateBootstrapMetrics(req.Name, prevPhase, inventory)

	return res, nil
}

func (r *Reconciler) validatorConfig() validation.Config {
	cfg := validation.Config{
		WorkloadsNamespace: common.WorkloadsNamespace,
		ValidatorApp:       common.AppName(common.ComponentValidator),
		GFDApp:             common.AppName(common.ComponentGPUFeatureDiscovery),
		DCGMApp:            common.AppName(common.ComponentDCGM),
		DCGMExporterApp:    common.AppName(common.ComponentDCGMExporter),
	}
	return cfg
}

func (r *Reconciler) updateBootstrapMetrics(node string, prevPhase string, inventory *v1alpha1.GPUNodeState) {
	newPhase := effectiveBootstrapPhase(inventory)
	if prevPhase != "" && prevPhase != newPhase {
		bootmetrics.BootstrapPhaseDelete(node, prevPhase)
	}
	bootmetrics.BootstrapPhaseSet(node, newPhase)

	for _, cond := range bootstrapConditionTypes {
		bootmetrics.BootstrapConditionSet(node, cond, conditionTrue(inventory, cond))
	}
}

func (r *Reconciler) clearBootstrapMetrics(node string) {
	for _, phase := range bootstrapPhases {
		bootmetrics.BootstrapPhaseDelete(node, phase)
	}
	for _, cond := range bootstrapConditionTypes {
		bootmetrics.BootstrapConditionDelete(node, cond)
	}
}

func effectiveBootstrapPhase(inventory *v1alpha1.GPUNodeState) string {
	if conditionTrue(inventory, conditionReadyForPooling) {
		return "Ready"
	}
	if !conditionTrue(inventory, conditionDriverReady) || !conditionTrue(inventory, conditionToolkitReady) {
		return "Validating"
	}
	if !conditionTrue(inventory, conditionMonitoringReady) {
		return "Monitoring"
	}
	return "Validating"
}

func conditionTrue(inventory *v1alpha1.GPUNodeState, condType string) bool {
	for _, cond := range inventory.Status.Conditions {
		if cond.Type == condType {
			return cond.Status == metav1.ConditionTrue
		}
	}
	return false
}
