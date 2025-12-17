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
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	moduleconfigctrl "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controllerbuilder"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
	cpmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/reconciler"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/validation"
)

const (
	controllerName             = "gpu-bootstrap-controller"
	cacheSyncTimeoutDuration   = 10 * time.Minute
	conditionInventoryComplete = "InventoryComplete"
	conditionDriverReady       = "DriverReady"
	conditionToolkitReady      = "ToolkitReady"
	conditionMonitoringReady   = "MonitoringReady"
	conditionReadyForPooling   = "ReadyForPooling"
	conditionWorkloadsDegraded = "WorkloadsDegraded"
)

var managedComponentSet = func() map[string]struct{} {
	set := make(map[string]struct{})
	for _, name := range meta.ComponentAppNames() {
		set[name] = struct{}{}
	}
	return set
}()

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

type Reconciler struct {
	client                 client.Client
	scheme                 *runtime.Scheme
	log                    logr.Logger
	cfg                    config.ControllerConfig
	store                  *config.ModuleConfigStore
	handlers               []contracts.BootstrapHandler
	builders               func(ctrl.Manager) controllerbuilder.Builder
	moduleWatcherFactory   func(cache.Cache, controllerbuilder.Builder) controllerbuilder.Builder
	workloadWatcherFactory func(cache.Cache, controllerbuilder.Builder) controllerbuilder.Builder
	validator              validation.Validator
}

func New(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore, handlers []contracts.BootstrapHandler) *Reconciler {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	rec := &Reconciler{
		log:      log,
		cfg:      cfg,
		store:    store,
		handlers: handlers,
		builders: controllerbuilder.NewManagedBy,
	}
	rec.moduleWatcherFactory = func(c cache.Cache, b controllerbuilder.Builder) controllerbuilder.Builder {
		return rec.attachModuleWatcher(b, c)
	}
	rec.workloadWatcherFactory = func(c cache.Cache, b controllerbuilder.Builder) controllerbuilder.Builder {
		return rec.attachWorkloadWatcher(b, c)
	}
	return rec
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	r.client = mgr.GetClient()
	r.injectClient()
	r.scheme = mgr.GetScheme()

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

	options := controller.Options{
		MaxConcurrentReconciles: r.cfg.Workers,
		RecoverPanic:            ptr.To(true),
		LogConstructor:          logger.NewConstructor(r.log),
		CacheSyncTimeout:        cacheSyncTimeoutDuration,
		NewQueue:                reconciler.NewNamedQueue(reconciler.UsePriorityQueue()),
	}

	ctrlBuilder := r.builders(mgr).
		Named(controllerName).
		For(&v1alpha1.GPUNodeState{}).
		WithOptions(options)

	if cache := mgr.GetCache(); cache != nil {
		if r.moduleWatcherFactory != nil {
			ctrlBuilder = r.moduleWatcherFactory(cache, ctrlBuilder)
		}
		if r.workloadWatcherFactory != nil {
			ctrlBuilder = r.workloadWatcherFactory(cache, ctrlBuilder)
		}
		ctrlBuilder = r.attachDeviceWatcher(ctrlBuilder, cache)
	}

	return ctrlBuilder.Complete(r)
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

func (r *Reconciler) attachModuleWatcher(b controllerbuilder.Builder, c cache.Cache) controllerbuilder.Builder {
	moduleConfig := &unstructured.Unstructured{}
	moduleConfig.SetGroupVersionKind(moduleconfigctrl.ModuleConfigGVK)
	handlerFunc := handler.TypedEnqueueRequestsFromMapFunc(r.mapModuleConfig)
	return b.WatchesRawSource(source.Kind(c, moduleConfig, handlerFunc))
}

func (r *Reconciler) attachWorkloadWatcher(b controllerbuilder.Builder, c cache.Cache) controllerbuilder.Builder {
	pod := &corev1.Pod{}
	handlerFunc := handler.TypedEnqueueRequestsFromMapFunc(mapWorkloadPodToInventory)
	return b.WatchesRawSource(source.Kind(c, pod, handlerFunc))
}

func (r *Reconciler) attachDeviceWatcher(b controllerbuilder.Builder, c cache.Cache) controllerbuilder.Builder {
	dev := &v1alpha1.GPUDevice{}
	handlerFunc := handler.TypedEnqueueRequestsFromMapFunc(mapDeviceToInventory)
	return b.WatchesRawSource(source.Kind(c, dev, handlerFunc, devicePredicates()))
}

func (r *Reconciler) mapModuleConfig(ctx context.Context, _ *unstructured.Unstructured) []reconcile.Request {
	if r.store != nil && !r.store.Current().Enabled {
		return nil
	}
	return r.requeueAllInventories(ctx)
}

func mapWorkloadPodToInventory(_ context.Context, pod *corev1.Pod) []reconcile.Request {
	if pod == nil {
		return nil
	}
	if pod.Namespace != meta.WorkloadsNamespace {
		return nil
	}
	if pod.Spec.NodeName == "" {
		return nil
	}
	if pod.Labels == nil {
		return nil
	}
	if _, ok := managedComponentSet[pod.Labels["app"]]; !ok {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: pod.Spec.NodeName}}}
}

func mapDeviceToInventory(_ context.Context, dev *v1alpha1.GPUDevice) []reconcile.Request {
	if dev == nil {
		return nil
	}

	nodeName := strings.TrimSpace(dev.Status.NodeName)
	if nodeName == "" && dev.Labels != nil {
		nodeName = strings.TrimSpace(dev.Labels["gpu.deckhouse.io/node"])
	}
	if nodeName == "" && dev.Labels != nil {
		nodeName = strings.TrimSpace(dev.Labels["kubernetes.io/hostname"])
	}
	if nodeName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: nodeName}}}
}

func devicePredicates() predicate.TypedPredicate[*v1alpha1.GPUDevice] {
	return predicate.TypedFuncs[*v1alpha1.GPUDevice]{
		CreateFunc:  func(event.TypedCreateEvent[*v1alpha1.GPUDevice]) bool { return true },
		DeleteFunc:  func(event.TypedDeleteEvent[*v1alpha1.GPUDevice]) bool { return true },
		GenericFunc: func(event.TypedGenericEvent[*v1alpha1.GPUDevice]) bool { return false },
		UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha1.GPUDevice]) bool {
			oldDev, newDev := e.ObjectOld, e.ObjectNew
			if oldDev == nil || newDev == nil {
				return true
			}
			return deviceChanged(oldDev, newDev)
		},
	}
}

func deviceChanged(oldDev, newDev *v1alpha1.GPUDevice) bool {
	if oldDev.Status.State != newDev.Status.State || oldDev.Status.NodeName != newDev.Status.NodeName || oldDev.Status.Managed != newDev.Status.Managed {
		return true
	}
	if (oldDev.Status.PoolRef == nil) != (newDev.Status.PoolRef == nil) {
		return true
	}
	if oldDev.Status.PoolRef != nil && newDev.Status.PoolRef != nil {
		if oldDev.Status.PoolRef.Name != newDev.Status.PoolRef.Name || oldDev.Status.PoolRef.Namespace != newDev.Status.PoolRef.Namespace {
			return true
		}
	}
	return false
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := crlog.FromContext(ctx).WithValues("inventory", req.Name)
	ctx = logr.NewContext(ctx, log)

	inventory := &v1alpha1.GPUNodeState{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: req.Name}, inventory); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(2).Info("GPUNodeState removed")
			r.clearBootstrapMetrics(req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if r.store != nil && !r.store.Current().Enabled {
		log.V(1).Info("module disabled, skipping bootstrap reconciliation")
		r.clearBootstrapMetrics(req.Name)
		return ctrl.Result{}, nil
	}

	if r.validator == nil {
		r.validator = validation.NewValidator(r.client, r.validatorConfig())
	}
	resource := reconciler.NewResource(inventory, r.client)
	prevPhase := effectiveBootstrapPhase(resource.Original())

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

	rec := reconciler.NewBase(r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, handler contracts.BootstrapHandler) (contracts.Result, error) {
		result, err := handler.HandleNode(ctx, inventory)
		if err != nil {
			cpmetrics.BootstrapHandlerErrorInc(handler.Name())
		}
		return result, err
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		if equality.Semantic.DeepEqual(resource.Original().Status, inventory.Status) {
			return nil
		}
		return resource.PatchStatus(ctx)
	})

	res, err := rec.Reconcile(ctx)
	if err != nil {
		log.Error(err, "handler chain failed")
		return ctrl.Result{}, err
	}
	r.updateBootstrapMetrics(req.Name, prevPhase, inventory)

	return ctrl.Result{
		Requeue:      res.Requeue,
		RequeueAfter: res.RequeueAfter,
	}, nil
}

func (r *Reconciler) requeueAllInventories(ctx context.Context) []reconcile.Request {
	if r.client == nil {
		return nil
	}
	list := &v1alpha1.GPUNodeStateList{}
	if err := r.client.List(ctx, list); err != nil {
		if r.log.GetSink() != nil {
			r.log.Error(err, "list GPUNodeState to resync after module config change")
		}
		return nil
	}

	requests := make([]reconcile.Request, 0, len(list.Items))
	for _, item := range list.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: item.Name}})
	}
	return requests
}

func (r *Reconciler) validatorConfig() validation.Config {
	cfg := validation.Config{
		WorkloadsNamespace: meta.WorkloadsNamespace,
		ValidatorApp:       meta.AppName(meta.ComponentValidator),
		GFDApp:             meta.AppName(meta.ComponentGPUFeatureDiscovery),
		DCGMApp:            meta.AppName(meta.ComponentDCGM),
		DCGMExporterApp:    meta.AppName(meta.ComponentDCGMExporter),
	}
	return cfg
}

func (r *Reconciler) updateBootstrapMetrics(node string, prevPhase string, inventory *v1alpha1.GPUNodeState) {
	newPhase := effectiveBootstrapPhase(inventory)
	if prevPhase != "" && prevPhase != newPhase {
		cpmetrics.BootstrapPhaseDelete(node, prevPhase)
	}
	cpmetrics.BootstrapPhaseSet(node, newPhase)

	for _, cond := range bootstrapConditionTypes {
		cpmetrics.BootstrapConditionSet(node, cond, conditionTrue(inventory, cond))
	}
}

func (r *Reconciler) clearBootstrapMetrics(node string) {
	for _, phase := range bootstrapPhases {
		cpmetrics.BootstrapPhaseDelete(node, phase)
	}
	for _, cond := range bootstrapConditionTypes {
		cpmetrics.BootstrapConditionDelete(node, cond)
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
