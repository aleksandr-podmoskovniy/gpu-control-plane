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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	moduleconfigctrl "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/controllers/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controllerbuilder"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/indexer"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
	cpmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/reconciler"
	moduleconfigpkg "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

const (
	controllerName           = "gpu-inventory-controller"
	cacheSyncTimeoutDuration = 10 * time.Minute

	conditionInventoryComplete = "InventoryComplete"

	reasonInventorySynced     = "InventorySynced"
	reasonNoDevicesDiscovered = "NoDevicesDiscovered"
	reasonNodeFeatureMissing  = "NodeFeatureMissing"

	eventDeviceDetected    = "GPUDeviceDetected"
	eventDeviceRemoved     = "GPUDeviceRemoved"
	eventInventoryChanged  = "GPUInventoryConditionChanged"
	eventDetectUnavailable = "GPUDetectionUnavailable"

	defaultResyncPeriod time.Duration = 0

	nodeFeatureNodeNameLabel = "nfd.node.kubernetes.io/node-name"
)

var (
	knownDeviceStates = []v1alpha1.GPUDeviceState{
		v1alpha1.GPUDeviceStateDiscovered,
		v1alpha1.GPUDeviceStateValidating,
		v1alpha1.GPUDeviceStateReady,
		v1alpha1.GPUDeviceStatePendingAssignment,
		v1alpha1.GPUDeviceStateAssigned,
		v1alpha1.GPUDeviceStateReserved,
		v1alpha1.GPUDeviceStateInUse,
		v1alpha1.GPUDeviceStateFaulted,
	}
)

var defaultBuilderFactory = controllerbuilder.NewManagedBy

var nodeFeatureSourceBuilder = func(cache cache.Cache) source.SyncingSource {
	obj := &nfdv1alpha1.NodeFeature{}
	obj.SetGroupVersionKind(nfdv1alpha1.SchemeGroupVersion.WithKind("NodeFeature"))

	return source.Kind(
		cache,
		obj,
		handler.TypedEnqueueRequestsFromMapFunc(mapNodeFeatureToNode),
		nodeFeaturePredicates(),
	)
}

var nodeStateSourceBuilder = func(cache cache.Cache) source.SyncingSource {
	obj := &v1alpha1.GPUNodeState{}

	return source.Kind(
		cache,
		obj,
		handler.TypedEnqueueRequestsFromMapFunc(mapNodeStateToNode),
		nodeStatePredicates(),
	)
}

type setupDependencies struct {
	client            client.Client
	scheme            *runtime.Scheme
	recorder          record.EventRecorder
	indexer           client.FieldIndexer
	cache             cache.Cache
	nodeFeatureSource source.SyncingSource
	nodeStateSource   source.SyncingSource
	builder           controllerbuilder.Builder
}

func defaultNodeFeatureSource(cache cache.Cache) source.SyncingSource {
	return nodeFeatureSourceBuilder(cache)
}

func defaultNodeStateSource(cache cache.Cache) source.SyncingSource {
	return nodeStateSourceBuilder(cache)
}

type Reconciler struct {
	client                   client.Client
	scheme                   *runtime.Scheme
	log                      logr.Logger
	cfg                      config.ControllerConfig
	handlers                 []contracts.InventoryHandler
	recorder                 record.EventRecorder
	resyncPeriod             time.Duration
	resyncMu                 sync.RWMutex
	builderFactory           func(ctrl.Manager) controllerbuilder.Builder
	nodeFeatureSourceFactory func(cache.Cache) source.SyncingSource
	nodeStateSourceFactory   func(cache.Cache) source.SyncingSource
	moduleWatcherFactory     func(cache.Cache, controllerbuilder.Builder) controllerbuilder.Builder
	store                    *config.ModuleConfigStore
	fallbackManaged          ManagedNodesPolicy
	fallbackApproval         DeviceApprovalPolicy
	detectionCollector       DetectionCollector
	cleanupService           CleanupService
	deviceService            DeviceService
	inventoryService         InventoryService
	detectionClient          client.Client
}

func (r *Reconciler) detectionSvc() DetectionCollector {
	if r.detectionCollector == nil || r.detectionClient != r.client {
		r.detectionCollector = newDetectionCollector(r.client)
		r.detectionClient = r.client
	}
	return r.detectionCollector
}

func (r *Reconciler) cleanupSvc() CleanupService {
	if r.cleanupService == nil {
		r.cleanupService = newCleanupService(r.client, r.recorder)
	}
	return r.cleanupService
}

func (r *Reconciler) deviceSvc() DeviceService {
	if r.deviceService == nil {
		r.deviceService = newDeviceService(r.client, r.scheme, r.recorder, r.handlers)
	}
	return r.deviceService
}

func (r *Reconciler) inventorySvc() InventoryService {
	if r.inventoryService == nil {
		r.inventoryService = newInventoryService(r.client, r.scheme, r.recorder)
	}
	return r.inventoryService
}

func (r *Reconciler) collectNodeDetections(ctx context.Context, node string) (nodeDetection, error) {
	return r.detectionSvc().Collect(ctx, node)
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
		builderFactory:           controllerbuilder.NewManagedBy,
		nodeFeatureSourceFactory: defaultNodeFeatureSource,
		nodeStateSourceFactory:   defaultNodeStateSource,
		store:                    store,
		fallbackManaged:          managed,
		fallbackApproval:         approval,
	}
	rec.setResyncPeriod(cfg.ResyncPeriod)
	rec.applyInventoryResync(state)
	rec.moduleWatcherFactory = func(c cache.Cache, b controllerbuilder.Builder) controllerbuilder.Builder {
		return rec.attachModuleWatcher(b, c)
	}

	return rec, nil
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if r.builderFactory == nil {
		r.builderFactory = defaultBuilderFactory
	}
	if r.nodeFeatureSourceFactory == nil {
		r.nodeFeatureSourceFactory = defaultNodeFeatureSource
	}
	if r.nodeStateSourceFactory == nil {
		r.nodeStateSourceFactory = defaultNodeStateSource
	}
	cache := mgr.GetCache()
	deps := setupDependencies{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		recorder:          mgr.GetEventRecorderFor("gpu-inventory-controller"),
		indexer:           mgr.GetFieldIndexer(),
		cache:             cache,
		nodeFeatureSource: r.nodeFeatureSourceFactory(cache),
		nodeStateSource:   r.nodeStateSourceFactory(cache),
		builder:           r.builderFactory(mgr),
	}
	return r.setupWithDependencies(ctx, deps)
}

func (r *Reconciler) setupWithDependencies(ctx context.Context, deps setupDependencies) error {
	r.client = deps.client
	r.scheme = deps.scheme
	r.recorder = deps.recorder
	if r.detectionCollector == nil {
		r.detectionCollector = newDetectionCollector(r.client)
	}
	if r.cleanupService == nil {
		r.cleanupService = newCleanupService(r.client, r.recorder)
	}
	if r.deviceService == nil {
		r.deviceService = newDeviceService(r.client, r.scheme, r.recorder, r.handlers)
	}
	if r.inventoryService == nil {
		r.inventoryService = newInventoryService(r.client, r.scheme, r.recorder)
	}

	predicates := r.nodePredicates()

	if err := indexer.IndexGPUDeviceByNode(ctx, deps.indexer); err != nil {
		return err
	}

	options := controller.Options{
		MaxConcurrentReconciles: r.cfg.Workers,
		RecoverPanic:            ptr.To(true),
		LogConstructor:          logger.NewConstructor(r.log),
		CacheSyncTimeout:        cacheSyncTimeoutDuration,
		NewQueue:                reconciler.NewNamedQueue(reconciler.UsePriorityQueue()),
	}

	ctrlBuilder := deps.builder.
		Named(controllerName).
		For(&corev1.Node{}, builder.WithPredicates(predicates)).
		WatchesRawSource(deps.nodeFeatureSource).
		WithOptions(options)

	// Recreate inventories on manual deletion without subscribing to all status updates.
	if deps.cache != nil && deps.nodeStateSource != nil {
		ctrlBuilder = ctrlBuilder.WatchesRawSource(deps.nodeStateSource)
	}

	if deps.cache != nil && r.moduleWatcherFactory != nil {
		ctrlBuilder = r.moduleWatcherFactory(deps.cache, ctrlBuilder)
	}

	return ctrlBuilder.Complete(r)
}

func (r *Reconciler) requeueAllNodes(ctx context.Context) []reconcile.Request {
	nodeList := &corev1.NodeList{}
	if err := r.client.List(ctx, nodeList, client.MatchingLabels{"gpu.deckhouse.io/present": "true"}); err != nil {
		if r.log.GetSink() != nil {
			r.log.Error(err, "list GPU nodes to resync after module config change")
		}
		return nil
	}
	requests := make([]reconcile.Request, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		nodeName := nodeList.Items[i].Name
		if nodeName == "" {
			continue
		}
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}})
	}
	sort.Slice(requests, func(i, j int) bool {
		return requests[i].Name < requests[j].Name
	})
	return requests
}

func (r *Reconciler) attachModuleWatcher(b controllerbuilder.Builder, cache cache.Cache) controllerbuilder.Builder {
	moduleConfig := &unstructured.Unstructured{}
	moduleConfig.SetGroupVersionKind(moduleconfigctrl.ModuleConfigGVK)
	handlerFunc := handler.TypedEnqueueRequestsFromMapFunc(r.mapModuleConfig)
	return b.WatchesRawSource(source.Kind(cache, moduleConfig, handlerFunc))
}

func (r *Reconciler) mapModuleConfig(ctx context.Context, _ *unstructured.Unstructured) []reconcile.Request {
	if r.store != nil && !r.store.Current().Enabled {
		return nil
	}
	r.refreshInventorySettings()
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
			// Rely on ownerReferences GC; avoid aggressive cleanup that may fire on transient cache misses.
			log.V(1).Info("node removed, skipping reconciliation")
			r.cleanupSvc().ClearMetrics(req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	nodeFeature, err := r.findNodeFeature(ctx, node.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	state := newInventoryState(node, nodeFeature, managedPolicy)
	nodeSnapshot := state.Snapshot()
	snapshotList := nodeSnapshot.Devices
	managed := nodeSnapshot.Managed

	// If NodeFeature data has not arrived yet and we have no device snapshots,
	// avoid deleting existing GPUDevice/GPUNodeState. We'll be requeued by the
	// NodeFeature watch once it appears.
	if !nodeSnapshot.FeatureDetected && len(snapshotList) == 0 {
		log.V(1).Info("node feature not detected yet, skip reconcile")
		return ctrl.Result{}, nil
	}

	allowCleanup := state.AllowCleanup()
	var orphanDevices map[string]struct{}
	if allowCleanup {
		if orphanDevices, err = state.OrphanDevices(ctx, r.client); err != nil {
			return ctrl.Result{}, err
		}
	}

	reconciledDevices := make([]*v1alpha1.GPUDevice, 0, len(snapshotList))
	aggregate := contracts.Result{}

	var detections nodeDetection
	if d, err := state.CollectDetections(ctx, r.collectNodeDetections); err == nil {
		detections = d
	} else {
		log.V(1).Info("gfd-extender telemetry unavailable", "node", node.Name, "error", err)
		r.recorder.Eventf(node, corev1.EventTypeWarning, eventDetectUnavailable, "gfd-extender unavailable for node %s: %v", node.Name, err)
	}

	for _, snapshot := range snapshotList {
		device, res, err := r.deviceSvc().Reconcile(ctx, node, snapshot, nodeSnapshot.Labels, managed, approvalPolicy, detections)
		if err != nil {
			return ctrl.Result{}, err
		}
		if orphanDevices != nil {
			delete(orphanDevices, device.Name)
		}
		reconciledDevices = append(reconciledDevices, device)
		aggregate = contracts.MergeResult(aggregate, res)
	}

	if node.GetDeletionTimestamp() != nil {
		if err := r.cleanupSvc().RemoveOrphans(ctx, node, orphanDevices); err != nil {
			return ctrl.Result{}, err
		}
	}

	ctrlResult := ctrl.Result{}
	if err := r.inventorySvc().Reconcile(ctx, node, nodeSnapshot, reconciledDevices); err != nil {
		return ctrl.Result{}, err
	}
	r.inventorySvc().UpdateDeviceMetrics(node.Name, reconciledDevices)

	hasDevices := len(reconciledDevices) > 0
	if hasDevices && aggregate.Requeue {
		ctrlResult.Requeue = true
	}
	if hasDevices && aggregate.RequeueAfter > 0 {
		if ctrlResult.RequeueAfter == 0 || aggregate.RequeueAfter < ctrlResult.RequeueAfter {
			ctrlResult.RequeueAfter = aggregate.RequeueAfter
		}
	}

	if ctrlResult.Requeue || ctrlResult.RequeueAfter > 0 {
		log.V(1).Info("inventory reconcile scheduled follow-up", "requeue", ctrlResult.Requeue, "after", ctrlResult.RequeueAfter)
	} else {
		log.V(2).Info("inventory reconcile completed", "devices", len(reconciledDevices))
	}

	return ctrlResult, nil
}

func (r *Reconciler) applyInventoryResync(state moduleconfigpkg.State) {
	if state.Inventory.ResyncPeriod == "" {
		return
	}
	duration, err := time.ParseDuration(state.Inventory.ResyncPeriod)
	if err != nil || duration <= 0 {
		return
	}
	r.setResyncPeriod(duration)
}

func (r *Reconciler) refreshInventorySettings() {
	if r.store == nil {
		return
	}
	state := r.store.Current()
	r.applyInventoryResync(state)
}

func (r *Reconciler) setResyncPeriod(period time.Duration) {
	r.resyncMu.Lock()
	r.resyncPeriod = period
	r.resyncMu.Unlock()
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
	if feature == nil {
		return nil
	}
	if !hasGPUDeviceLabels(feature.Spec.Labels) {
		return nil
	}

	nodeName := feature.GetName()
	if labeled := feature.GetLabels()[nodeFeatureNodeNameLabel]; labeled != "" {
		nodeName = labeled
	}
	nodeName = strings.TrimPrefix(nodeName, "nvidia-features-for-")
	if nodeName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: nodeName}}}
}

func mapNodeStateToNode(ctx context.Context, state *v1alpha1.GPUNodeState) []reconcile.Request {
	_ = ctx
	if state == nil {
		return nil
	}
	nodeName := strings.TrimSpace(state.Name)
	if nodeName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: nodeName}}}
}

func nodeStatePredicates() predicate.TypedPredicate[*v1alpha1.GPUNodeState] {
	return predicate.TypedFuncs[*v1alpha1.GPUNodeState]{
		CreateFunc:  func(event.TypedCreateEvent[*v1alpha1.GPUNodeState]) bool { return false },
		UpdateFunc:  func(event.TypedUpdateEvent[*v1alpha1.GPUNodeState]) bool { return false },
		DeleteFunc:  func(event.TypedDeleteEvent[*v1alpha1.GPUNodeState]) bool { return true },
		GenericFunc: func(event.TypedGenericEvent[*v1alpha1.GPUNodeState]) bool { return false },
	}
}

func boolToConditionStatus(value bool) metav1.ConditionStatus {
	if value {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func nodeFeaturePredicates() predicate.TypedPredicate[*nfdv1alpha1.NodeFeature] {
	return predicate.TypedFuncs[*nfdv1alpha1.NodeFeature]{
		CreateFunc: func(e event.TypedCreateEvent[*nfdv1alpha1.NodeFeature]) bool {
			return hasGPUDeviceLabels(e.Object.Spec.Labels)
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*nfdv1alpha1.NodeFeature]) bool {
			oldLabels := nodeFeatureLabels(e.ObjectOld)
			newLabels := nodeFeatureLabels(e.ObjectNew)
			oldHas := hasGPUDeviceLabels(oldLabels)
			newHas := hasGPUDeviceLabels(newLabels)
			if !oldHas && !newHas {
				return false
			}
			if oldHas != newHas {
				return true
			}
			return gpuLabelsDiffer(oldLabels, newLabels)
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*nfdv1alpha1.NodeFeature]) bool {
			return hasGPUDeviceLabels(nodeFeatureLabels(e.Object))
		},
		GenericFunc: func(event.TypedGenericEvent[*nfdv1alpha1.NodeFeature]) bool { return false },
	}
}

func nodeFeatureLabels(feature *nfdv1alpha1.NodeFeature) map[string]string {
	if feature == nil {
		return nil
	}
	return feature.Spec.Labels
}

func updateDeviceStateMetrics(nodeName string, devices []*v1alpha1.GPUDevice) {
	counts := make(map[string]int, len(devices))
	for _, device := range devices {
		stateKey := string(normalizeDeviceState(device.Status.State))
		counts[stateKey]++
	}
	seen := make(map[string]struct{}, len(counts))
	for state, count := range counts {
		cpmetrics.InventoryDeviceStateSet(nodeName, state, count)
		seen[state] = struct{}{}
	}
	for _, state := range knownDeviceStates {
		key := string(state)
		if _, ok := seen[key]; !ok {
			cpmetrics.InventoryDeviceStateDelete(nodeName, key)
		}
	}
}

func normalizeDeviceState(state v1alpha1.GPUDeviceState) v1alpha1.GPUDeviceState {
	if state == "" {
		return v1alpha1.GPUDeviceStateDiscovered
	}
	return state
}

func nodeLabels(node *corev1.Node) map[string]string {
	if node == nil {
		return nil
	}
	return node.Labels
}

func nodeHasGPUHardwareLabels(labels map[string]string) bool {
	return hasGPUDeviceLabels(labels)
}

func gpuNodeLabelsChanged(oldNode, newNode *corev1.Node) bool {
	oldLabels := nodeLabels(oldNode)
	newLabels := nodeLabels(newNode)

	oldHas := nodeHasGPUHardwareLabels(oldLabels)
	newHas := nodeHasGPUHardwareLabels(newLabels)
	if oldHas != newHas {
		return true
	}

	if gpuLabelsDiffer(oldLabels, newLabels) {
		return true
	}

	return false
}

func hasGPUDeviceLabels(labels map[string]string) bool {
	if len(labels) == 0 {
		return false
	}
	for key := range labels {
		if strings.HasPrefix(key, deviceLabelPrefix) || strings.HasPrefix(key, migProfileLabelPrefix) {
			return true
		}
	}
	if labels[gfdProductLabel] != "" ||
		labels[gfdMemoryLabel] != "" ||
		labels[gfdMigCapableLabel] != "" ||
		labels[gfdMigAltCapableLabel] != "" ||
		labels[gfdMigStrategyLabel] != "" ||
		labels[gfdMigAltStrategy] != "" {
		return true
	}
	return false
}

func gpuLabelsDiffer(oldLabels, newLabels map[string]string) bool {
	relevantKey := func(key string) bool {
		if strings.HasPrefix(key, deviceLabelPrefix) || strings.HasPrefix(key, migProfileLabelPrefix) {
			return true
		}
		switch key {
		case gfdProductLabel,
			gfdMemoryLabel,
			gfdComputeMajorLabel,
			gfdComputeMinorLabel,
			gfdDriverVersionLabel,
			gfdCudaRuntimeVersionLabel,
			gfdCudaDriverMajorLabel,
			gfdCudaDriverMinorLabel,
			gfdMigCapableLabel,
			gfdMigStrategyLabel,
			gfdMigAltCapableLabel,
			gfdMigAltStrategy:
			return true
		default:
			return false
		}
	}

	get := func(labels map[string]string, key string) string {
		if labels == nil {
			return ""
		}
		return labels[key]
	}

	for key, val := range oldLabels {
		if !relevantKey(key) {
			continue
		}
		if val != get(newLabels, key) {
			return true
		}
	}
	for key, val := range newLabels {
		if !relevantKey(key) {
			continue
		}
		if val != get(oldLabels, key) {
			return true
		}
	}
	return false
}
