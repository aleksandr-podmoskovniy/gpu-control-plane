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
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/reconciler"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/pkg/apis/nfd/v1alpha1"
)

const (
	controllerName           = "gpu-inventory-controller"
	cacheSyncTimeoutDuration = 10 * time.Minute

	conditionManagedDisabled     = "ManagedDisabled"
	conditionInventoryIncomplete = "InventoryIncomplete"

	reasonNodeManagedEnabled  = "NodeManaged"
	reasonNodeManagedDisabled = "NodeMarkedDisabled"
	reasonInventorySynced     = "InventorySynced"
	reasonNoDevicesDiscovered = "NoDevicesDiscovered"
	reasonNodeFeatureMissing  = "NodeFeatureMissing"

	eventDeviceDetected   = "GPUDeviceDetected"
	eventDeviceRemoved    = "GPUDeviceRemoved"
	eventInventoryChanged = "GPUInventoryConditionChanged"

	defaultResyncPeriod = 30 * time.Second
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
)

var newControllerManagedBy = func(mgr ctrl.Manager) controllerRuntimeAdapter {
	return &controllerRuntimeWrapper{builder: ctrl.NewControllerManagedBy(mgr)}
}

var nodeFeatureSourceBuilder = func(cache cache.Cache) source.SyncingSource {
	return source.Kind(
		cache,
		&nfdv1alpha1.NodeFeature{},
		handler.TypedEnqueueRequestsFromMapFunc(mapNodeFeatureToNode),
	)
}

func init() {
	metrics.Registry.MustRegister(inventoryDevicesGauge, inventoryConditionGauge)
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
	managed                  ManagedNodesPolicy
	approval                 DeviceApprovalPolicy
	builderFactory           func(ctrl.Manager) controllerBuilder
	nodeFeatureSourceFactory func(cache.Cache) source.SyncingSource
}

func New(log logr.Logger, cfg config.ControllerConfig, module config.ModuleSettings, handlers []contracts.InventoryHandler) (*Reconciler, error) {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.ResyncPeriod <= 0 {
		cfg.ResyncPeriod = defaultResyncPeriod
	}

	approval, err := newDeviceApprovalPolicy(module.DeviceApproval)
	if err != nil {
		return nil, err
	}

	return &Reconciler{
		log:                      log,
		cfg:                      cfg,
		handlers:                 handlers,
		resyncPeriod:             cfg.ResyncPeriod,
		builderFactory:           defaultControllerBuilder,
		nodeFeatureSourceFactory: defaultNodeFeatureSource,
		managed: ManagedNodesPolicy{
			LabelKey:         module.ManagedNodes.LabelKey,
			EnabledByDefault: module.ManagedNodes.EnabledByDefault,
		},
		approval: approval,
	}, nil
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if r.builderFactory == nil {
		r.builderFactory = defaultControllerBuilder
	}
	if r.nodeFeatureSourceFactory == nil {
		r.nodeFeatureSourceFactory = defaultNodeFeatureSource
	}
	deps := setupDependencies{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		recorder:          mgr.GetEventRecorderFor("gpu-inventory-controller"),
		indexer:           mgr.GetFieldIndexer(),
		nodeFeatureSource: r.nodeFeatureSourceFactory(mgr.GetCache()),
		builder:           r.builderFactory(mgr),
	}
	return r.setupWithDependencies(ctx, deps)
}

func (r *Reconciler) setupWithDependencies(ctx context.Context, deps setupDependencies) error {
	r.client = deps.client
	r.scheme = deps.scheme
	r.recorder = deps.recorder

	if err := deps.indexer.IndexField(ctx, &gpuv1alpha1.GPUDevice{}, deviceNodeIndexKey, func(obj client.Object) []string {
		device, ok := obj.(*gpuv1alpha1.GPUDevice)
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
		Owns(&gpuv1alpha1.GPUDevice{}).
		Owns(&gpuv1alpha1.GPUNodeInventory{}).
		WatchesRawSource(deps.nodeFeatureSource).
		WithOptions(options)

	return builder.Complete(r)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := crlog.FromContext(ctx).WithValues("node", req.Name)
	ctx = logr.NewContext(ctx, log)

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

	var nodeFeature *nfdv1alpha1.NodeFeature
	feature := &nfdv1alpha1.NodeFeature{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: node.Name}, feature); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	} else {
		nodeFeature = feature
	}

	nodeSnapshot := buildNodeSnapshot(node, nodeFeature, r.managed)
	snapshotList := nodeSnapshot.Devices
	managed := nodeSnapshot.Managed

	existingDevices := &gpuv1alpha1.GPUDeviceList{}
	if err := r.client.List(ctx, existingDevices, client.MatchingFields{deviceNodeIndexKey: node.Name}); err != nil {
		return ctrl.Result{}, err
	}
	orphanDevices := make(map[string]struct{}, len(existingDevices.Items))
	for i := range existingDevices.Items {
		orphanDevices[existingDevices.Items[i].Name] = struct{}{}
	}

	reconciledDevices := make([]*gpuv1alpha1.GPUDevice, 0, len(snapshotList))
	aggregate := contracts.Result{}

	for _, snapshot := range snapshotList {
		device, res, err := r.reconcileDevice(ctx, node, snapshot, nodeSnapshot.Labels, managed)
		if err != nil {
			return ctrl.Result{}, err
		}
		delete(orphanDevices, device.Name)
		reconciledDevices = append(reconciledDevices, device)
		aggregate = contracts.MergeResult(aggregate, res)
	}

	for name := range orphanDevices {
		if err := r.client.Delete(ctx, &gpuv1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: name}}); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		log.V(1).Info("removed orphan GPUDevice", "device", name)
		r.recorder.Eventf(node, corev1.EventTypeNormal, eventDeviceRemoved, "GPU device %s removed from inventory", name)
	}

	if err := r.reconcileNodeInventory(ctx, node, nodeSnapshot, reconciledDevices); err != nil {
		return ctrl.Result{}, err
	}
	inventoryDevicesGauge.WithLabelValues(node.Name).Set(float64(len(reconciledDevices)))

	ctrlResult := ctrl.Result{}
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

func (r *Reconciler) reconcileDevice(ctx context.Context, node *corev1.Node, snapshot deviceSnapshot, nodeLabels map[string]string, managed bool) (*gpuv1alpha1.GPUDevice, contracts.Result, error) {
	deviceName := buildDeviceName(node.Name, snapshot)
	device := &gpuv1alpha1.GPUDevice{}
	err := r.client.Get(ctx, types.NamespacedName{Name: deviceName}, device)
	if apierrors.IsNotFound(err) {
		return r.createDevice(ctx, node, snapshot, nodeLabels, managed)
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
	autoAttach := r.approval.AutoAttach(managed, labelsForDevice(snapshot, nodeLabels))
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

func (r *Reconciler) createDevice(ctx context.Context, node *corev1.Node, snapshot deviceSnapshot, nodeLabels map[string]string, managed bool) (*gpuv1alpha1.GPUDevice, contracts.Result, error) {
	device := &gpuv1alpha1.GPUDevice{
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
	device.Status.State = gpuv1alpha1.GPUDeviceStateUnassigned
	device.Status.AutoAttach = r.approval.AutoAttach(managed, labelsForDevice(snapshot, nodeLabels))

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

func (r *Reconciler) ensureDeviceMetadata(ctx context.Context, node *corev1.Node, device *gpuv1alpha1.GPUDevice, snapshot deviceSnapshot) (bool, error) {
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

func (r *Reconciler) invokeHandlers(ctx context.Context, device *gpuv1alpha1.GPUDevice) (contracts.Result, error) {
	log := crlog.FromContext(ctx).WithValues("device", device.Name)
	ctx = logr.NewContext(ctx, log)

	rec := reconciler.NewBase(r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, handler contracts.InventoryHandler) (contracts.Result, error) {
		return handler.HandleDevice(ctx, device)
	})
	rec.SetResourceUpdater(func(context.Context) error { return nil })

	return rec.Reconcile(ctx)
}

func (r *Reconciler) reconcileNodeInventory(ctx context.Context, node *corev1.Node, snapshot nodeSnapshot, devices []*gpuv1alpha1.GPUDevice) error {
	inventory := &gpuv1alpha1.GPUNodeInventory{}
	err := r.client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory)
	if apierrors.IsNotFound(err) {
		inventory = &gpuv1alpha1.GPUNodeInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name: node.Name,
			},
			Spec: gpuv1alpha1.GPUNodeInventorySpec{
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

	hardware := gpuv1alpha1.GPUNodeHardware{
		Present: len(devices) > 0,
		Devices: make([]gpuv1alpha1.GPUNodeDevice, 0, len(devices)),
	}
	snapshotByIndex := make(map[string]deviceSnapshot, len(snapshot.Devices))
	for _, snap := range snapshot.Devices {
		snapshotByIndex[snap.Index] = snap
	}
	for _, device := range devices {
		index := device.Labels[deviceIndexLabelKey]
		snap, ok := snapshotByIndex[index]

		nodeDevice := gpuv1alpha1.GPUNodeDevice{
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
	labelKey := r.managed.LabelKey
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

	inventoryIncomplete := !snapshot.FeatureDetected || len(snapshot.Devices) == 0
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
	incompleteCond := metav1.Condition{
		Type:               conditionInventoryIncomplete,
		Status:             boolToConditionStatus(inventoryIncomplete),
		Reason:             inventoryReason,
		Message:            inventoryMessage,
		ObservedGeneration: inventory.Generation,
	}
	inventoryChanged := setStatusCondition(&conditions, incompleteCond)
	inventoryConditionGauge.WithLabelValues(node.Name, conditionInventoryIncomplete).Set(boolToFloat(inventoryIncomplete))

	inventory.Status.Conditions = conditions
	if managedChanged {
		r.recorder.Eventf(node, corev1.EventTypeNormal, eventInventoryChanged, "Condition %s changed to %t (%s)", conditionManagedDisabled, !snapshot.Managed, managedReason)
	}
	if inventoryChanged {
		eventType := corev1.EventTypeNormal
		if inventoryIncomplete {
			eventType = corev1.EventTypeWarning
		}
		r.recorder.Eventf(node, eventType, eventInventoryChanged, "Condition %s changed to %t (%s)", conditionInventoryIncomplete, inventoryIncomplete, inventoryReason)
	}

	if !equality.Semantic.DeepEqual(statusBefore.Status, inventory.Status) {
		if err := r.client.Status().Patch(ctx, inventory, client.MergeFrom(statusBefore)); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) cleanupNode(ctx context.Context, nodeName string) error {
	deviceList := &gpuv1alpha1.GPUDeviceList{}
	if err := r.client.List(ctx, deviceList, client.MatchingFields{deviceNodeIndexKey: nodeName}); err != nil {
		return err
	}
	for i := range deviceList.Items {
		device := &gpuv1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Name: deviceList.Items[i].Name}}
		if err := r.client.Delete(ctx, device); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	if err := r.client.Delete(ctx, &gpuv1alpha1.GPUNodeInventory{ObjectMeta: metav1.ObjectMeta{Name: nodeName}}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	inventoryDevicesGauge.DeleteLabelValues(nodeName)
	inventoryConditionGauge.DeleteLabelValues(nodeName, conditionManagedDisabled)
	inventoryConditionGauge.DeleteLabelValues(nodeName, conditionInventoryIncomplete)

	return nil
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

func capabilityFromSnapshot(snapshot deviceSnapshot) *gpuv1alpha1.GPUComputeCapability {
	if snapshot.ComputeMajor == 0 && snapshot.ComputeMinor == 0 {
		return nil
	}
	return &gpuv1alpha1.GPUComputeCapability{Major: snapshot.ComputeMajor, Minor: snapshot.ComputeMinor}
}

func computeCapabilityEqual(left, right *gpuv1alpha1.GPUComputeCapability) bool {
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
