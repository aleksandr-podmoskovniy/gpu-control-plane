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
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/components"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/state"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	moduleconfigctrl "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/reconciler"
)

type controllerBuilder interface {
	Named(string) controllerBuilder
	For(client.Object, ...builder.ForOption) controllerBuilder
	WithOptions(controller.Options) controllerBuilder
	WatchesRawSource(source.Source) controllerBuilder
	Complete(reconcile.Reconciler) error
}

type controllerRuntimeAdapter interface {
	Named(string) controllerRuntimeAdapter
	For(client.Object, ...builder.ForOption) controllerRuntimeAdapter
	WithOptions(controller.Options) controllerRuntimeAdapter
	WatchesRawSource(source.Source) controllerRuntimeAdapter
	Complete(reconcile.Reconciler) error
}

type runtimeControllerBuilder struct {
	adapter controllerRuntimeAdapter
}

func (b *runtimeControllerBuilder) Named(name string) controllerBuilder {
	b.adapter = b.adapter.Named(name)
	return b
}

func (b *runtimeControllerBuilder) For(obj client.Object, opts ...builder.ForOption) controllerBuilder {
	b.adapter = b.adapter.For(obj, opts...)
	return b
}

func (b *runtimeControllerBuilder) WithOptions(opts controller.Options) controllerBuilder {
	b.adapter = b.adapter.WithOptions(opts)
	return b
}

func (b *runtimeControllerBuilder) WatchesRawSource(src source.Source) controllerBuilder {
	b.adapter = b.adapter.WatchesRawSource(src)
	return b
}

func (b *runtimeControllerBuilder) Complete(r reconcile.Reconciler) error {
	return b.adapter.Complete(r)
}

type builderControllerAdapter struct {
	delegate *builder.Builder
}

func (a *builderControllerAdapter) Named(name string) controllerRuntimeAdapter {
	a.delegate = a.delegate.Named(name)
	return a
}

func (a *builderControllerAdapter) For(obj client.Object, opts ...builder.ForOption) controllerRuntimeAdapter {
	a.delegate = a.delegate.For(obj, opts...)
	return a
}

func (a *builderControllerAdapter) WithOptions(opts controller.Options) controllerRuntimeAdapter {
	a.delegate = a.delegate.WithOptions(opts)
	return a
}

func (a *builderControllerAdapter) WatchesRawSource(src source.Source) controllerRuntimeAdapter {
	a.delegate = a.delegate.WatchesRawSource(src)
	return a
}

func (a *builderControllerAdapter) Complete(r reconcile.Reconciler) error {
	return a.delegate.Complete(r)
}

const (
	controllerName             = "gpu-bootstrap-controller"
	cacheSyncTimeoutDuration   = 10 * time.Minute
	conditionManagedDisabled   = "ManagedDisabled"
	conditionReadyForPooling   = "ReadyForPooling"
	conditionDriverMissing     = "DriverMissing"
	conditionToolkitMissing    = "ToolkitMissing"
	conditionMonitoringMissing = "MonitoringMissing"
	conditionGFDReady          = "GFDReady"
	conditionInventoryComplete = "InventoryComplete"
)

var managedComponentSet = func() map[string]struct{} {
	set := make(map[string]struct{})
	for _, name := range meta.ComponentAppNames() {
		set[name] = struct{}{}
	}
	return set
}()

var (
	bootstrapPhaseGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "gpu",
			Subsystem: "bootstrap",
			Name:      "node_phase",
			Help:      "Current bootstrap phase per node.",
		},
		[]string{"node", "phase"},
	)
	bootstrapConditionGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "gpu",
			Subsystem: "bootstrap",
			Name:      "condition",
			Help:      "Bootstrap conditions that are true for a node.",
		},
		[]string{"node", "condition"},
	)
	bootstrapHandlerErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gpu",
			Subsystem: "bootstrap",
			Name:      "handler_errors_total",
			Help:      "Number of bootstrap handler failures.",
		},
		[]string{"handler"},
	)
	bootstrapPhases = []v1alpha1.GPUNodeBootstrapPhase{
		v1alpha1.GPUNodeBootstrapPhaseDisabled,
		v1alpha1.GPUNodeBootstrapPhaseValidating,
		v1alpha1.GPUNodeBootstrapPhaseValidatingFailed,
		v1alpha1.GPUNodeBootstrapPhaseGFD,
		v1alpha1.GPUNodeBootstrapPhaseMonitoring,
		v1alpha1.GPUNodeBootstrapPhaseReady,
	}
	bootstrapConditionTypes = []string{
		conditionReadyForPooling,
		conditionDriverMissing,
		conditionToolkitMissing,
		conditionMonitoringMissing,
		conditionGFDReady,
	}
)

func init() {
	metrics.Registry.MustRegister(
		bootstrapPhaseGauge,
		bootstrapConditionGauge,
		bootstrapHandlerErrors,
	)
}

var newControllerManagedBy = func(mgr ctrl.Manager) controllerBuilder {
	return &runtimeControllerBuilder{
		adapter: &builderControllerAdapter{delegate: ctrl.NewControllerManagedBy(mgr)},
	}
}

type Reconciler struct {
	client                 client.Client
	scheme                 *runtime.Scheme
	log                    logr.Logger
	cfg                    config.ControllerConfig
	store                  *config.ModuleConfigStore
	stateStore             *state.Store
	handlers               []contracts.BootstrapHandler
	builders               func(ctrl.Manager) controllerBuilder
	moduleWatcherFactory   func(cache.Cache, controllerBuilder) controllerBuilder
	workloadWatcherFactory func(cache.Cache, controllerBuilder) controllerBuilder
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
		builders: newControllerManagedBy,
	}
	rec.moduleWatcherFactory = func(c cache.Cache, builder controllerBuilder) controllerBuilder {
		return rec.attachModuleWatcher(builder, c)
	}
	rec.workloadWatcherFactory = func(c cache.Cache, builder controllerBuilder) controllerBuilder {
		return rec.attachWorkloadWatcher(builder, c)
	}
	return rec
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	r.client = mgr.GetClient()
	r.injectClient()
	r.scheme = mgr.GetScheme()
	r.stateStore = state.NewStore(
		mgr.GetClient(),
		mgr.GetAPIReader(),
		meta.WorkloadsNamespace,
		meta.StateConfigMapName,
		types.NamespacedName{
			Name:      meta.ControllerDeploymentName,
			Namespace: meta.WorkloadsNamespace,
		},
	)
	if err := r.stateStore.Ensure(ctx); err != nil {
		return fmt.Errorf("ensure bootstrap state configmap: %w", err)
	}

	options := controller.Options{
		MaxConcurrentReconciles: r.cfg.Workers,
		RecoverPanic:            ptr.To(true),
		LogConstructor:          logger.NewConstructor(r.log),
		CacheSyncTimeout:        cacheSyncTimeoutDuration,
	}

	builder := r.builders(mgr).
		Named(controllerName).
		For(&v1alpha1.GPUNodeInventory{}).
		WithOptions(options)

	if cache := mgr.GetCache(); cache != nil {
		if r.moduleWatcherFactory != nil {
			builder = r.moduleWatcherFactory(cache, builder)
		}
		if r.workloadWatcherFactory != nil {
			builder = r.workloadWatcherFactory(cache, builder)
		}
	}

	return builder.Complete(r)
}

func (r *Reconciler) injectClient() {
	if r.client == nil {
		return
	}
	for _, handler := range r.handlers {
		if setter, ok := handler.(interface{ SetClient(client.Client) }); ok {
			setter.SetClient(r.client)
		}
	}
}

func (r *Reconciler) attachModuleWatcher(builder controllerBuilder, c cache.Cache) controllerBuilder {
	moduleConfig := &unstructured.Unstructured{}
	moduleConfig.SetGroupVersionKind(moduleconfigctrl.ModuleConfigGVK)
	handlerFunc := handler.TypedEnqueueRequestsFromMapFunc[*unstructured.Unstructured](r.mapModuleConfig)
	return builder.WatchesRawSource(source.Kind(c, moduleConfig, handlerFunc))
}

func (r *Reconciler) attachWorkloadWatcher(builder controllerBuilder, c cache.Cache) controllerBuilder {
	pod := &corev1.Pod{}
	handlerFunc := handler.TypedEnqueueRequestsFromMapFunc[*corev1.Pod](mapWorkloadPodToInventory)
	return builder.WatchesRawSource(source.Kind(c, pod, handlerFunc))
}

func (r *Reconciler) mapModuleConfig(ctx context.Context, _ *unstructured.Unstructured) []reconcile.Request {
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

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := crlog.FromContext(ctx).WithValues("inventory", req.Name)
	ctx = logr.NewContext(ctx, log)

	inventory := &v1alpha1.GPUNodeInventory{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: req.Name}, inventory); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(2).Info("GPUNodeInventory removed")
			r.deleteNodeState(ctx, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	original := inventory.DeepCopy()
	prevPhase := effectiveBootstrapPhase(original)

	rec := reconciler.NewBase(r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, handler contracts.BootstrapHandler) (contracts.Result, error) {
		result, err := handler.HandleNode(ctx, inventory)
		if err != nil {
			bootstrapHandlerErrors.WithLabelValues(handler.Name()).Inc()
		}
		return result, err
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		if equality.Semantic.DeepEqual(original.Status, inventory.Status) {
			return nil
		}
		return r.client.Status().Patch(ctx, inventory, client.MergeFrom(original))
	})

	res, err := rec.Reconcile(ctx)
	if err != nil {
		log.Error(err, "handler chain failed")
		return ctrl.Result{}, err
	}
	r.persistNodeState(ctx, inventory)
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
	list := &v1alpha1.GPUNodeInventoryList{}
	if err := r.client.List(ctx, list); err != nil {
		if r.log.GetSink() != nil {
			r.log.Error(err, "list GPUNodeInventory to resync after module config change")
		}
		return nil
	}

	requests := make([]reconcile.Request, 0, len(list.Items))
	for _, item := range list.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: item.Name}})
	}
	return requests
}

func (r *Reconciler) persistNodeState(ctx context.Context, inventory *v1alpha1.GPUNodeInventory) {
	if r.stateStore == nil {
		return
	}
	phase := effectiveBootstrapPhase(inventory)
	componentSet := map[string]bool{}
	if inventoryReadyForBootstrap(inventory) {
		enabled := components.EnabledComponents(phase)
		componentSet = make(map[string]bool, len(enabled))
		for component := range enabled {
			componentSet[string(component)] = true
		}
	}
	nodeState := state.NodeState{
		Phase:      string(phase),
		Components: componentSet,
	}
	if err := r.stateStore.UpdateNode(ctx, inventory.Name, nodeState); err != nil {
		r.log.Error(err, "failed to update bootstrap state", "node", inventory.Name)
	}
}

func (r *Reconciler) deleteNodeState(ctx context.Context, node string) {
	if r.stateStore == nil {
		return
	}
	r.clearBootstrapMetrics(node)
	if err := r.stateStore.DeleteNode(ctx, node); err != nil {
		r.log.Error(err, "failed to drop bootstrap state", "node", node)
	}
}

func (r *Reconciler) updateBootstrapMetrics(node string, prevPhase v1alpha1.GPUNodeBootstrapPhase, inventory *v1alpha1.GPUNodeInventory) {
	newPhase := effectiveBootstrapPhase(inventory)
	if prevPhase != "" && prevPhase != newPhase {
		bootstrapPhaseGauge.DeleteLabelValues(node, string(prevPhase))
	}
	bootstrapPhaseGauge.WithLabelValues(node, string(newPhase)).Set(1)

	for _, cond := range bootstrapConditionTypes {
		if conditionTrue(inventory, cond) {
			bootstrapConditionGauge.WithLabelValues(node, cond).Set(1)
		} else {
			bootstrapConditionGauge.DeleteLabelValues(node, cond)
		}
	}
}

func (r *Reconciler) clearBootstrapMetrics(node string) {
	for _, phase := range bootstrapPhases {
		bootstrapPhaseGauge.DeleteLabelValues(node, string(phase))
	}
	for _, cond := range bootstrapConditionTypes {
		bootstrapConditionGauge.DeleteLabelValues(node, cond)
	}
}

func effectiveBootstrapPhase(inventory *v1alpha1.GPUNodeInventory) v1alpha1.GPUNodeBootstrapPhase {
	phase := inventory.Status.Bootstrap.Phase
	if phase == "" {
		phase = v1alpha1.GPUNodeBootstrapPhaseValidating
	}
	if isManagedDisabled(inventory) {
		return v1alpha1.GPUNodeBootstrapPhaseDisabled
	}
	return phase
}

func isManagedDisabled(inventory *v1alpha1.GPUNodeInventory) bool {
	cond := apimeta.FindStatusCondition(inventory.Status.Conditions, conditionManagedDisabled)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

func inventoryReadyForBootstrap(inventory *v1alpha1.GPUNodeInventory) bool {
	cond := apimeta.FindStatusCondition(inventory.Status.Conditions, conditionInventoryComplete)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

func conditionTrue(inventory *v1alpha1.GPUNodeInventory, condType string) bool {
	cond := apimeta.FindStatusCondition(inventory.Status.Conditions, condType)
	return cond != nil && cond.Status == metav1.ConditionTrue
}
