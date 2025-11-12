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
	"fmt"
	"sort"
	"strconv"
	"strings"
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
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	moduleconfigctrl "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/reconciler"
	moduleconfigpkg "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

const (
	controllerName           = "gpu-inventory-controller"
	cacheSyncTimeoutDuration = 10 * time.Minute

	conditionManagedDisabled   = "ManagedDisabled"
	conditionInventoryComplete = "InventoryComplete"

	reasonNodeManagedEnabled  = "NodeManaged"
	reasonNodeManagedDisabled = "NodeMarkedDisabled"
	reasonInventorySynced     = "InventorySynced"
	reasonNoDevicesDiscovered = "NoDevicesDiscovered"
	reasonNodeFeatureMissing  = "NodeFeatureMissing"

	eventDeviceDetected   = "GPUDeviceDetected"
	eventDeviceRemoved    = "GPUDeviceRemoved"
	eventInventoryChanged = "GPUInventoryConditionChanged"

	defaultResyncPeriod = 30 * time.Second

	nodeFeatureNodeNameLabel = "nfd.node.kubernetes.io/node-name"
)

var (
	inventoryDevicesGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpu",
		Subsystem: "inventory",
		Name:      "devices_total",
		Help:      "Number of GPU devices discovered on a node.",
	}, []string{"node"})

	inventoryConditionGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpu",
		Subsystem: "inventory",
		Name:      "condition",
		Help:      "Inventory condition status (0 or 1).",
	}, []string{"node", "condition"})

	inventoryDeviceStateGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "gpu",
		Subsystem: "inventory",
		Name:      "devices_state",
		Help:      "Number of GPU devices on a node grouped by state.",
	}, []string{"node", "state"})

	inventoryHandlerErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gpu",
		Subsystem: "inventory",
		Name:      "handler_errors_total",
		Help:      "Number of errors returned by inventory handlers.",
	}, []string{"handler"})

	knownDeviceStates = []v1alpha1.GPUDeviceState{
		v1alpha1.GPUDeviceStateDiscovered,
		v1alpha1.GPUDeviceStateReadyForPooling,
		v1alpha1.GPUDeviceStatePendingAssignment,
		v1alpha1.GPUDeviceStateNoPoolMatched,
		v1alpha1.GPUDeviceStateAssigned,
		v1alpha1.GPUDeviceStateReserved,
		v1alpha1.GPUDeviceStateInUse,
		v1alpha1.GPUDeviceStateFaulted,
		// legacy value kept for metrics cleanup while migrating away from the old enum.
		v1alpha1.GPUDeviceStateUnassigned,
	}
)

var newControllerManagedBy = func(mgr ctrl.Manager) controllerRuntimeAdapter {
	return &controllerRuntimeWrapper{builder: ctrl.NewControllerManagedBy(mgr)}
}

var nodeFeatureSourceBuilder = func(cache cache.Cache) source.SyncingSource {
	obj := &nfdv1alpha1.NodeFeature{}
	obj.SetGroupVersionKind(nfdv1alpha1.SchemeGroupVersion.WithKind("NodeFeature"))

	return source.Kind(
		cache,
		obj,
		handler.TypedEnqueueRequestsFromMapFunc(mapNodeFeatureToNode),
	)
}

func init() {
	metrics.Registry.MustRegister(
		inventoryDevicesGauge,
		inventoryConditionGauge,
		inventoryDeviceStateGauge,
		inventoryHandlerErrors,
	)
}

type controllerBuilder interface {
	Named(string) controllerBuilder
	For(client.Object, ...builder.ForOption) controllerBuilder
	Owns(client.Object, ...builder.OwnsOption) controllerBuilder
	WatchesRawSource(source.Source) controllerBuilder
	WithOptions(controller.Options) controllerBuilder
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

func (b *runtimeControllerBuilder) Owns(obj client.Object, opts ...builder.OwnsOption) controllerBuilder {
	b.adapter = b.adapter.Owns(obj, opts...)
	return b
}

func (b *runtimeControllerBuilder) WatchesRawSource(src source.Source) controllerBuilder {
	b.adapter = b.adapter.WatchesRawSource(src)
	return b
}

func (b *runtimeControllerBuilder) WithOptions(opts controller.Options) controllerBuilder {
	b.adapter = b.adapter.WithOptions(opts)
	return b
}

func (b *runtimeControllerBuilder) Complete(r reconcile.Reconciler) error {
	return b.adapter.Complete(r)
}

type controllerRuntimeAdapter interface {
	Named(string) controllerRuntimeAdapter
	For(client.Object, ...builder.ForOption) controllerRuntimeAdapter
	Owns(client.Object, ...builder.OwnsOption) controllerRuntimeAdapter
	WatchesRawSource(source.Source) controllerRuntimeAdapter
	WithOptions(controller.Options) controllerRuntimeAdapter
	Complete(reconcile.Reconciler) error
}

type controllerRuntimeWrapper struct {
	builder *builder.Builder
}

func (w *controllerRuntimeWrapper) Named(name string) controllerRuntimeAdapter {
	w.builder = w.builder.Named(name)
	return w
}

func (w *controllerRuntimeWrapper) For(obj client.Object, opts ...builder.ForOption) controllerRuntimeAdapter {
	w.builder = w.builder.For(obj, opts...)
	return w
}

func (w *controllerRuntimeWrapper) Owns(obj client.Object, opts ...builder.OwnsOption) controllerRuntimeAdapter {
	w.builder = w.builder.Owns(obj, opts...)
	return w
}

func (w *controllerRuntimeWrapper) WatchesRawSource(src source.Source) controllerRuntimeAdapter {
	w.builder = w.builder.WatchesRawSource(src)
	return w
}

func (w *controllerRuntimeWrapper) WithOptions(opts controller.Options) controllerRuntimeAdapter {
	w.builder = w.builder.WithOptions(opts)
	return w
}

func (w *controllerRuntimeWrapper) Complete(r reconcile.Reconciler) error {
	return w.builder.Complete(r)
}

type setupDependencies struct {
	client            client.Client
	scheme            *runtime.Scheme
	recorder          record.EventRecorder
	indexer           client.FieldIndexer
	cache             cache.Cache
	nodeFeatureSource source.SyncingSource
	builder           controllerBuilder
}

func defaultControllerBuilder(mgr ctrl.Manager) controllerBuilder {
	return &runtimeControllerBuilder{adapter: newControllerManagedBy(mgr)}
}

func defaultNodeFeatureSource(cache cache.Cache) source.SyncingSource {
	return nodeFeatureSourceBuilder(cache)
}

type Reconciler struct {
	client                   client.Client
	scheme                   *runtime.Scheme
	log                      logr.Logger
	cfg                      config.ControllerConfig
	handlers                 []contracts.InventoryHandler
	recorder                 record.EventRecorder
	resyncPeriod             time.Duration
	builderFactory           func(ctrl.Manager) controllerBuilder
	nodeFeatureSourceFactory func(cache.Cache) source.SyncingSource
	moduleWatcherFactory     func(cache.Cache, controllerBuilder) controllerBuilder
	store                    *config.ModuleConfigStore
	fallbackManaged          ManagedNodesPolicy
	fallbackApproval         DeviceApprovalPolicy
}

func New(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore, handlers []contracts.InventoryHandler) (*Reconciler, error) {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.ResyncPeriod <= 0 {
		cfg.ResyncPeriod = defaultResyncPeriod
	}

	state := moduleconfigpkg.DefaultState()
	if store != nil {
		state = store.Current()
	}

	managed, approval, err := managedAndApprovalFromState(state)
	if err != nil {
		return nil, err
	}

	rec := &Reconciler{
		log:                      log,
		cfg:                      cfg,
		handlers:                 handlers,
		resyncPeriod:             cfg.ResyncPeriod,
		builderFactory:           defaultControllerBuilder,
		nodeFeatureSourceFactory: defaultNodeFeatureSource,
		store:                    store,
		fallbackManaged:          managed,
		fallbackApproval:         approval,
	}
	rec.moduleWatcherFactory = func(c cache.Cache, builder controllerBuilder) controllerBuilder {
		return rec.attachModuleWatcher(builder, c)
	}

	return rec, nil
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if r.builderFactory == nil {
		r.builderFactory = defaultControllerBuilder
	}
	if r.nodeFeatureSourceFactory == nil {
		r.nodeFeatureSourceFactory = defaultNodeFeatureSource
	}
	cache := mgr.GetCache()
	deps := setupDependencies{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		recorder:          mgr.GetEventRecorderFor("gpu-inventory-controller"),
		indexer:           mgr.GetFieldIndexer(),
		cache:             cache,
		nodeFeatureSource: r.nodeFeatureSourceFactory(cache),
		builder:           r.builderFactory(mgr),
	}
	return r.setupWithDependencies(ctx, deps)
}

func (r *Reconciler) setupWithDependencies(ctx context.Context, deps setupDependencies) error {
	r.client = deps.client
	r.scheme = deps.scheme
	r.recorder = deps.recorder

	if err := deps.indexer.IndexField(ctx, &v1alpha1.GPUDevice{}, deviceNodeIndexKey, func(obj client.Object) []string {
		device, ok := obj.(*v1alpha1.GPUDevice)
		if !ok {
			return nil
		}
		if device.Status.NodeName == "" {
			return nil
		}
		return []string{device.Status.NodeName}
	}); err != nil {
		return err
	}

	options := controller.Options{
		MaxConcurrentReconciles: r.cfg.Workers,
		RecoverPanic:            ptr.To(true),
		LogConstructor:          logger.NewConstructor(r.log),
		CacheSyncTimeout:        cacheSyncTimeoutDuration,
	}

	builder := deps.builder.
		Named(controllerName).
		For(&corev1.Node{}).
		Owns(&v1alpha1.GPUDevice{}).
		Owns(&v1alpha1.GPUNodeInventory{}).
		WatchesRawSource(deps.nodeFeatureSource).
		WithOptions(options)

	if deps.cache != nil && r.moduleWatcherFactory != nil {
		builder = r.moduleWatcherFactory(deps.cache, builder)
	}

	return builder.Complete(r)
}

func (r *Reconciler) requeueAllNodes(ctx context.Context) []reconcile.Request {
	nodeList := &corev1.NodeList{}
	if err := r.client.List(ctx, nodeList); err != nil {
		if r.log.GetSink() != nil {
			r.log.Error(err, "list nodes to resync after module config change")
		}
		return nil
	}
	requests := make([]reconcile.Request, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
	}
	return requests
}

func (r *Reconciler) attachModuleWatcher(builder controllerBuilder, cache cache.Cache) controllerBuilder {
	moduleConfig := &unstructured.Unstructured{}
	moduleConfig.SetGroupVersionKind(moduleconfigctrl.ModuleConfigGVK)
	handlerFunc := handler.TypedEnqueueRequestsFromMapFunc[*unstructured.Unstructured](r.mapModuleConfig)
	return builder.WatchesRawSource(source.Kind(cache, moduleConfig, handlerFunc))
}

func (r *Reconciler) mapModuleConfig(ctx context.Context, _ *unstructured.Unstructured) []reconcile.Request {
	return r.requeueAllNodes(ctx)
}

func (r *Reconciler) currentPolicies() (ManagedNodesPolicy, DeviceApprovalPolicy) {
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

func managedAndApprovalFromState(state moduleconfigpkg.State) (ManagedNodesPolicy, DeviceApprovalPolicy, error) {
	managed := ManagedNodesPolicy{
		LabelKey:         strings.TrimSpace(state.Settings.ManagedNodes.LabelKey),
		EnabledByDefault: state.Settings.ManagedNodes.EnabledByDefault,
	}
	if managed.LabelKey == "" {
		managed.LabelKey = defaultManagedNodeLabelKey
	}

	approval, err := newDeviceApprovalPolicy(state.Settings.DeviceApproval)
	if err != nil {
		return ManagedNodesPolicy{}, DeviceApprovalPolicy{}, err
	}

	return managed, approval, nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := crlog.FromContext(ctx).WithValues("node", req.Name)
	ctx = logr.NewContext(ctx, log)

	managedPolicy, approvalPolicy := r.currentPolicies()

	node := &corev1.Node{}
	if err := r.client.Get(ctx, req.NamespacedName, node); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("node removed, cleaning inventory data")
			if err := r.cleanupNode(ctx, req.Name); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	nodeFeature, err := r.findNodeFeature(ctx, node.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	nodeSnapshot := buildNodeSnapshot(node, nodeFeature, managedPolicy)
	snapshotList := nodeSnapshot.Devices
	managed := nodeSnapshot.Managed

	existingDevices := &v1alpha1.GPUDeviceList{}
	if err := r.client.List(ctx, existingDevices, client.MatchingFields{deviceNodeIndexKey: node.Name}); err != nil {
		return ctrl.Result{}, err
	}
	orphanDevices := make(map[string]struct{}, len(existingDevices.Items))
	for i := range existingDevices.Items {
		orphanDevices[existingDevices.Items[i].Name] = struct{}{}
	}

	reconciledDevices := make([]*v1alpha1.GPUDevice, 0, len(snapshotList))
	aggregate := contracts.Result{}

	for _, snapshot := range snapshotList {
		device, res, err := r.reconcileDevice(ctx, node, snapshot, nodeSnapshot.Labels, managed, approvalPolicy)
		if err != nil {
			return ctrl.Result{}, err
		}
		delete(orphanDevices, device.Name)
		reconciledDevices = append(reconciledDevices, device)
		aggregate = contracts.MergeResult(aggregate, res)
	}

	for name := range orphanDevices {
		if err := r.client.Delete(ctx, &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: name}}); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		log.V(1).Info("removed orphan GPUDevice", "device", name)
		r.recorder.Eventf(node, corev1.EventTypeNormal, eventDeviceRemoved, "GPU device %s removed from inventory", name)
	}

	ctrlResult := ctrl.Result{}
	if len(reconciledDevices) == 0 {
		r.clearInventoryMetrics(node.Name)
		if err := r.deleteInventory(ctx, node.Name); err != nil {
			return ctrlResult, err
		}
		if r.resyncPeriod > 0 {
			ctrlResult.RequeueAfter = r.resyncPeriod
		}
		return ctrlResult, nil
	}

	if err := r.reconcileNodeInventory(ctx, node, nodeSnapshot, reconciledDevices, managedPolicy); err != nil {
		return ctrl.Result{}, err
	}
	updateDeviceStateMetrics(node.Name, reconciledDevices)
	inventoryDevicesGauge.WithLabelValues(node.Name).Set(float64(len(reconciledDevices)))

	if aggregate.Requeue {
		ctrlResult.Requeue = true
	}
	if aggregate.RequeueAfter > 0 {
		if ctrlResult.RequeueAfter == 0 || aggregate.RequeueAfter < ctrlResult.RequeueAfter {
			ctrlResult.RequeueAfter = aggregate.RequeueAfter
		}
	}
	if !ctrlResult.Requeue && ctrlResult.RequeueAfter == 0 && r.resyncPeriod > 0 {
		ctrlResult.RequeueAfter = r.resyncPeriod
	}

	if ctrlResult.Requeue || ctrlResult.RequeueAfter > 0 {
		log.V(1).Info("inventory reconcile scheduled follow-up", "requeue", ctrlResult.Requeue, "after", ctrlResult.RequeueAfter)
	} else {
		log.V(2).Info("inventory reconcile completed", "devices", len(reconciledDevices))
	}

	return ctrlResult, nil
}

func (r *Reconciler) findNodeFeature(ctx context.Context, nodeName string) (*nfdv1alpha1.NodeFeature, error) {
	feature := &nfdv1alpha1.NodeFeature{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: nodeName}, feature); err == nil {
		return feature, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}

	list := &nfdv1alpha1.NodeFeatureList{}
	if err := r.client.List(ctx, list, client.MatchingLabels{nodeFeatureNodeNameLabel: nodeName}); err != nil {
		return nil, err
	}

	return chooseNodeFeature(list.Items, nodeName), nil
}

func chooseNodeFeature(items []nfdv1alpha1.NodeFeature, nodeName string) *nfdv1alpha1.NodeFeature {
	if len(items) == 0 {
		return nil
	}

	selected := items[0].DeepCopy()
	for i := 1; i < len(items); i++ {
		item := items[i]
		if item.GetName() == nodeName && selected.GetName() != nodeName {
			selected = item.DeepCopy()
			continue
		}
		if resourceVersionNewer(item.GetResourceVersion(), selected.GetResourceVersion()) {
			selected = item.DeepCopy()
		}
	}
	return selected
}

func (r *Reconciler) reconcileDevice(ctx context.Context, node *corev1.Node, snapshot deviceSnapshot, nodeLabels map[string]string, managed bool, approval DeviceApprovalPolicy) (*v1alpha1.GPUDevice, contracts.Result, error) {
	deviceName := buildDeviceName(node.Name, snapshot)
	device := &v1alpha1.GPUDevice{}
	err := r.client.Get(ctx, types.NamespacedName{Name: deviceName}, device)
	if apierrors.IsNotFound(err) {
		return r.createDevice(ctx, node, snapshot, nodeLabels, managed, approval)
	}
	if err != nil {
		return nil, contracts.Result{}, err
	}

	metaUpdated, err := r.ensureDeviceMetadata(ctx, node, device, snapshot)
	if err != nil {
		return nil, contracts.Result{}, err
	}
	if metaUpdated {
		if err := r.client.Get(ctx, types.NamespacedName{Name: deviceName}, device); err != nil {
			return nil, contracts.Result{}, err
		}
	}

	statusBefore := device.DeepCopy()
	desiredInventoryID := buildInventoryID(node.Name, snapshot)

	if device.Status.NodeName != node.Name {
		device.Status.NodeName = node.Name
	}
	if device.Status.InventoryID != desiredInventoryID {
		device.Status.InventoryID = desiredInventoryID
	}
	if device.Status.Managed != managed {
		device.Status.Managed = managed
	}
	if device.Status.Hardware.PCI.Vendor != snapshot.Vendor ||
		device.Status.Hardware.PCI.Device != snapshot.Device ||
		device.Status.Hardware.PCI.Class != snapshot.Class {
		device.Status.Hardware.PCI.Vendor = snapshot.Vendor
		device.Status.Hardware.PCI.Device = snapshot.Device
		device.Status.Hardware.PCI.Class = snapshot.Class
	}
	if device.Status.Hardware.Product != snapshot.Product {
		device.Status.Hardware.Product = snapshot.Product
	}
	if device.Status.Hardware.MemoryMiB != snapshot.MemoryMiB {
		device.Status.Hardware.MemoryMiB = snapshot.MemoryMiB
	}
	desiredCapability := capabilityFromSnapshot(snapshot)
	if !computeCapabilityEqual(device.Status.Hardware.ComputeCapability, desiredCapability) {
		device.Status.Hardware.ComputeCapability = desiredCapability
	}
	if !equality.Semantic.DeepEqual(device.Status.Hardware.MIG, snapshot.MIG) {
		device.Status.Hardware.MIG = snapshot.MIG
	}
	if !stringSlicesEqual(device.Status.Hardware.Precision.Supported, snapshot.Precision) {
		device.Status.Hardware.Precision.Supported = append([]string(nil), snapshot.Precision...)
	}
	autoAttach := approval.AutoAttach(managed, labelsForDevice(snapshot, nodeLabels))
	if device.Status.AutoAttach != autoAttach {
		device.Status.AutoAttach = autoAttach
	}

	result, err := r.invokeHandlers(ctx, device)
	if err != nil {
		return nil, result, err
	}

	if !equality.Semantic.DeepEqual(statusBefore.Status, device.Status) {
		if err := r.client.Status().Patch(ctx, device, client.MergeFrom(statusBefore)); err != nil {
			if apierrors.IsConflict(err) {
				return device, contracts.MergeResult(result, contracts.Result{Requeue: true}), nil
			}
			return nil, result, err
		}
	}

	return device, result, nil
}

func (r *Reconciler) createDevice(ctx context.Context, node *corev1.Node, snapshot deviceSnapshot, nodeLabels map[string]string, managed bool, approval DeviceApprovalPolicy) (*v1alpha1.GPUDevice, contracts.Result, error) {
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: buildDeviceName(node.Name, snapshot),
			Labels: map[string]string{
				deviceNodeLabelKey:  node.Name,
				deviceIndexLabelKey: snapshot.Index,
			},
		},
	}
	if err := controllerutil.SetOwnerReference(node, device, r.scheme); err != nil {
		return nil, contracts.Result{}, err
	}

	if err := r.client.Create(ctx, device); err != nil {
		return nil, contracts.Result{}, err
	}
	r.recorder.Eventf(device, corev1.EventTypeNormal, eventDeviceDetected, "Discovered GPU device index=%s vendor=%s device=%s on node %s", snapshot.Index, snapshot.Vendor, snapshot.Device, node.Name)

	device.Status.NodeName = node.Name
	device.Status.InventoryID = buildInventoryID(node.Name, snapshot)
	device.Status.Managed = managed
	device.Status.Hardware.PCI.Vendor = snapshot.Vendor
	device.Status.Hardware.PCI.Device = snapshot.Device
	device.Status.Hardware.PCI.Class = snapshot.Class
	device.Status.Hardware.Product = snapshot.Product
	device.Status.Hardware.MemoryMiB = snapshot.MemoryMiB
	device.Status.Hardware.ComputeCapability = capabilityFromSnapshot(snapshot)
	device.Status.Hardware.MIG = snapshot.MIG
	device.Status.Hardware.Precision.Supported = append([]string(nil), snapshot.Precision...)
	device.Status.State = v1alpha1.GPUDeviceStateDiscovered
	device.Status.AutoAttach = approval.AutoAttach(managed, labelsForDevice(snapshot, nodeLabels))

	result, err := r.invokeHandlers(ctx, device)
	if err != nil {
		return nil, result, err
	}

	if err := r.client.Status().Update(ctx, device); err != nil {
		if apierrors.IsConflict(err) {
			return device, contracts.MergeResult(result, contracts.Result{Requeue: true}), nil
		}
		return nil, result, err
	}

	return device, result, nil
}

func (r *Reconciler) ensureDeviceMetadata(ctx context.Context, node *corev1.Node, device *v1alpha1.GPUDevice, snapshot deviceSnapshot) (bool, error) {
	desired := device.DeepCopy()
	changed := false

	if desired.Labels == nil {
		desired.Labels = make(map[string]string)
	}
	if desired.Labels[deviceNodeLabelKey] != node.Name {
		desired.Labels[deviceNodeLabelKey] = node.Name
		changed = true
	}
	if desired.Labels[deviceIndexLabelKey] != snapshot.Index {
		desired.Labels[deviceIndexLabelKey] = snapshot.Index
		changed = true
	}
	if err := controllerutil.SetOwnerReference(node, desired, r.scheme); err != nil {
		return false, err
	}
	if !equality.Semantic.DeepEqual(device.GetOwnerReferences(), desired.GetOwnerReferences()) {
		changed = true
	}

	if !changed {
		return false, nil
	}

	if err := r.client.Patch(ctx, desired, client.MergeFrom(device)); err != nil {
		return false, err
	}
	*device = *desired

	return true, nil
}

func (r *Reconciler) invokeHandlers(ctx context.Context, device *v1alpha1.GPUDevice) (contracts.Result, error) {
	log := crlog.FromContext(ctx).WithValues("device", device.Name)
	ctx = logr.NewContext(ctx, log)

	rec := reconciler.NewBase(r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, handler contracts.InventoryHandler) (contracts.Result, error) {
		result, err := handler.HandleDevice(ctx, device)
		if err != nil {
			inventoryHandlerErrors.WithLabelValues(handler.Name()).Inc()
		}
		return result, err
	})
	rec.SetResourceUpdater(func(context.Context) error { return nil })

	return rec.Reconcile(ctx)
}

func (r *Reconciler) reconcileNodeInventory(ctx context.Context, node *corev1.Node, snapshot nodeSnapshot, devices []*v1alpha1.GPUDevice, managedPolicy ManagedNodesPolicy) error {
	inventory := &v1alpha1.GPUNodeInventory{}
	err := r.client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory)
	if apierrors.IsNotFound(err) {
		inventory = &v1alpha1.GPUNodeInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name: node.Name,
			},
			Spec: v1alpha1.GPUNodeInventorySpec{
				NodeName: node.Name,
			},
		}
		if err := controllerutil.SetOwnerReference(node, inventory, r.scheme); err != nil {
			return err
		}
		if err := r.client.Create(ctx, inventory); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	specBefore := inventory.DeepCopy()
	changed := false

	if inventory.Spec.NodeName != node.Name {
		inventory.Spec.NodeName = node.Name
		changed = true
	}
	if err := controllerutil.SetOwnerReference(node, inventory, r.scheme); err != nil {
		return err
	}
	if !equality.Semantic.DeepEqual(specBefore.OwnerReferences, inventory.OwnerReferences) {
		changed = true
	}

	if changed {
		if err := r.client.Patch(ctx, inventory, client.MergeFrom(specBefore)); err != nil {
			return err
		}
		if err := r.client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
			return err
		}
	}

	hardware := v1alpha1.GPUNodeHardware{
		Present: len(devices) > 0,
		Devices: make([]v1alpha1.GPUNodeDevice, 0, len(devices)),
	}
	snapshotByIndex := make(map[string]deviceSnapshot, len(snapshot.Devices))
	for _, snap := range snapshot.Devices {
		snapshotByIndex[snap.Index] = snap
	}
	for _, device := range devices {
		index := device.Labels[deviceIndexLabelKey]
		snap, ok := snapshotByIndex[index]

		nodeDevice := v1alpha1.GPUNodeDevice{
			InventoryID: device.Status.InventoryID,
			Product:     device.Status.Hardware.Product,
			PCI:         device.Status.Hardware.PCI,
			MemoryMiB:   device.Status.Hardware.MemoryMiB,
			MIG:         device.Status.Hardware.MIG,
			ComputeCap:  device.Status.Hardware.ComputeCapability,
			Precision:   device.Status.Hardware.Precision,
			State:       device.Status.State,
			AutoAttach:  device.Status.AutoAttach,
			Health:      device.Status.Health,
		}
		if ok {
			if nodeDevice.Product == "" {
				nodeDevice.Product = snap.Product
			}
			if nodeDevice.MemoryMiB == 0 {
				nodeDevice.MemoryMiB = snap.MemoryMiB
			}
			if nodeDevice.ComputeCap == nil {
				nodeDevice.ComputeCap = capabilityFromSnapshot(snap)
			}
			if len(nodeDevice.Precision.Supported) == 0 && len(snap.Precision) > 0 {
				nodeDevice.Precision.Supported = append([]string(nil), snap.Precision...)
			}
			if snap.UUID != "" {
				nodeDevice.UUID = snap.UUID
			}
		}

		hardware.Devices = append(hardware.Devices, nodeDevice)
	}
	sort.Slice(hardware.Devices, func(i, j int) bool {
		return hardware.Devices[i].InventoryID < hardware.Devices[j].InventoryID
	})

	statusBefore := inventory.DeepCopy()
	inventory.Status.Hardware = hardware
	inventory.Status.Driver.Version = snapshot.Driver.Version
	inventory.Status.Driver.CUDAVersion = snapshot.Driver.CUDAVersion
	inventory.Status.Driver.ToolkitReady = snapshot.Driver.ToolkitInstalled || snapshot.Driver.ToolkitReady

	conditions := inventory.Status.Conditions
	labelKey := managedPolicy.LabelKey
	if labelKey == "" {
		labelKey = defaultManagedNodeLabelKey
	}
	managedMessage := "node managed by module"
	managedReason := reasonNodeManagedEnabled
	if !snapshot.Managed {
		managedMessage = fmt.Sprintf("node is marked with %s=false", labelKey)
		managedReason = reasonNodeManagedDisabled
	}
	managedCond := metav1.Condition{
		Type:               conditionManagedDisabled,
		Status:             boolToConditionStatus(!snapshot.Managed),
		Reason:             managedReason,
		Message:            managedMessage,
		ObservedGeneration: inventory.Generation,
	}
	managedChanged := setStatusCondition(&conditions, managedCond)
	inventoryConditionGauge.WithLabelValues(node.Name, conditionManagedDisabled).Set(boolToFloat(!snapshot.Managed))

	inventoryComplete := snapshot.FeatureDetected && len(snapshot.Devices) > 0
	inventoryReason := reasonInventorySynced
	inventoryMessage := "inventory data collected"
	switch {
	case !snapshot.FeatureDetected:
		inventoryReason = reasonNodeFeatureMissing
		inventoryMessage = "NodeFeature resource not discovered yet"
	case len(snapshot.Devices) == 0:
		inventoryReason = reasonNoDevicesDiscovered
		inventoryMessage = "no NVIDIA devices detected on the node"
	}
	completeCond := metav1.Condition{
		Type:               conditionInventoryComplete,
		Status:             boolToConditionStatus(inventoryComplete),
		Reason:             inventoryReason,
		Message:            inventoryMessage,
		ObservedGeneration: inventory.Generation,
	}
	inventoryChanged := setStatusCondition(&conditions, completeCond)
	inventoryConditionGauge.WithLabelValues(node.Name, conditionInventoryComplete).Set(boolToFloat(inventoryComplete))

	inventory.Status.Conditions = conditions
	if managedChanged {
		r.recorder.Eventf(node, corev1.EventTypeNormal, eventInventoryChanged, "Condition %s changed to %t (%s)", conditionManagedDisabled, !snapshot.Managed, managedReason)
	}
	if inventoryChanged {
		eventType := corev1.EventTypeNormal
		if !inventoryComplete {
			eventType = corev1.EventTypeWarning
		}
		r.recorder.Eventf(node, eventType, eventInventoryChanged, "Condition %s changed to %t (%s)", conditionInventoryComplete, inventoryComplete, inventoryReason)
	}

	if !equality.Semantic.DeepEqual(statusBefore.Status, inventory.Status) {
		if err := r.client.Status().Patch(ctx, inventory, client.MergeFrom(statusBefore)); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) deleteInventory(ctx context.Context, nodeName string) error {
	inventory := &v1alpha1.GPUNodeInventory{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: nodeName}, inventory); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := r.client.Delete(ctx, inventory); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *Reconciler) clearInventoryMetrics(nodeName string) {
	inventoryDevicesGauge.DeleteLabelValues(nodeName)
	inventoryConditionGauge.DeleteLabelValues(nodeName, conditionManagedDisabled)
	inventoryConditionGauge.DeleteLabelValues(nodeName, conditionInventoryComplete)
	for _, state := range knownDeviceStates {
		inventoryDeviceStateGauge.DeleteLabelValues(nodeName, string(state))
	}
}

func (r *Reconciler) cleanupNode(ctx context.Context, nodeName string) error {
	deviceList := &v1alpha1.GPUDeviceList{}
	if err := r.client.List(ctx, deviceList, client.MatchingFields{deviceNodeIndexKey: nodeName}); err != nil {
		return err
	}
	for i := range deviceList.Items {
		device := &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: deviceList.Items[i].Name}}
		if err := r.client.Delete(ctx, device); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	if err := r.deleteInventory(ctx, nodeName); err != nil {
		return err
	}
	r.clearInventoryMetrics(nodeName)

	return nil
}

func resourceVersionNewer(candidate, current string) bool {
	if candidate == "" {
		return false
	}
	if current == "" {
		return true
	}

	candidateInt, errCandidate := strconv.ParseUint(candidate, 10, 64)
	currentInt, errCurrent := strconv.ParseUint(current, 10, 64)

	switch {
	case errCandidate == nil && errCurrent == nil:
		return candidateInt > currentInt
	case errCandidate == nil:
		return true
	case errCurrent == nil:
		return false
	default:
		return candidate > current
	}
}

func mapNodeFeatureToNode(ctx context.Context, feature *nfdv1alpha1.NodeFeature) []reconcile.Request {
	_ = ctx
	nodeName := feature.GetName()
	if nodeName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: nodeName}}}
}

func boolToConditionStatus(value bool) metav1.ConditionStatus {
	if value {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func updateDeviceStateMetrics(nodeName string, devices []*v1alpha1.GPUDevice) {
	counts := make(map[string]float64, len(devices))
	for _, device := range devices {
		stateKey := string(normalizeDeviceState(device.Status.State))
		counts[stateKey]++
	}
	seen := make(map[string]struct{}, len(counts))
	for state, count := range counts {
		inventoryDeviceStateGauge.WithLabelValues(nodeName, state).Set(count)
		seen[state] = struct{}{}
	}
	for _, state := range knownDeviceStates {
		key := string(state)
		if _, ok := seen[key]; !ok {
			inventoryDeviceStateGauge.DeleteLabelValues(nodeName, key)
		}
	}
}

func normalizeDeviceState(state v1alpha1.GPUDeviceState) v1alpha1.GPUDeviceState {
	switch state {
	case "", v1alpha1.GPUDeviceStateUnassigned:
		return v1alpha1.GPUDeviceStateDiscovered
	default:
		return state
	}
}

func capabilityFromSnapshot(snapshot deviceSnapshot) *v1alpha1.GPUComputeCapability {
	if snapshot.ComputeMajor == 0 && snapshot.ComputeMinor == 0 {
		return nil
	}
	return &v1alpha1.GPUComputeCapability{Major: snapshot.ComputeMajor, Minor: snapshot.ComputeMinor}
}

func computeCapabilityEqual(left, right *v1alpha1.GPUComputeCapability) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return left.Major == right.Major && left.Minor == right.Minor
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func setStatusCondition(conditions *[]metav1.Condition, condition metav1.Condition) bool {
	prev := apimeta.FindStatusCondition(*conditions, condition.Type)
	var prevCopy *metav1.Condition
	if prev != nil {
		cpy := *prev
		prevCopy = &cpy
	}
	apimeta.SetStatusCondition(conditions, condition)
	next := apimeta.FindStatusCondition(*conditions, condition.Type)
	if prevCopy == nil {
		return next != nil
	}
	return prevCopy.Status != next.Status || prevCopy.Reason != next.Reason || prevCopy.Message != next.Message
}
