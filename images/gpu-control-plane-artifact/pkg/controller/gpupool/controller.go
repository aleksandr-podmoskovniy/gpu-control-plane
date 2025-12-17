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

package gpupool

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controllerbuilder"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
)

const (
	controllerName           = "gpu-pool-controller"
	cacheSyncTimeoutDuration = 10 * time.Minute
	assignmentAnnotation     = "gpu.deckhouse.io/assignment"
)

type Reconciler struct {
	client               client.Client
	scheme               *runtime.Scheme
	log                  logr.Logger
	cfg                  config.ControllerConfig
	store                *moduleconfig.ModuleConfigStore
	handlers             []contracts.PoolHandler
	builders             func(ctrl.Manager) controllerbuilder.Builder
	moduleWatcherFactory func(cache.Cache, controllerbuilder.Builder) controllerbuilder.Builder
}

func New(log logr.Logger, cfg config.ControllerConfig, store *moduleconfig.ModuleConfigStore, handlers []contracts.PoolHandler) *Reconciler {
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
	return rec
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	r.client = mgr.GetClient()
	r.scheme = mgr.GetScheme()

	if idx := mgr.GetFieldIndexer(); idx != nil {
		if err := indexer.IndexNodeByTaintKey(ctx, idx); err != nil {
			return err
		}
		if err := indexer.IndexGPUDeviceByPoolRefName(ctx, idx); err != nil {
			return err
		}
		if err := indexer.IndexGPUDeviceByNamespacedAssignment(ctx, idx); err != nil {
			return err
		}
		if err := indexer.IndexGPUDeviceByClusterAssignment(ctx, idx); err != nil {
			return err
		}
		if err := indexer.IndexGPUPoolByName(ctx, idx); err != nil {
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
		For(&v1alpha1.GPUPool{}, builder.WithPredicates(poolPredicates())).
		WithOptions(options)

	if cache := mgr.GetCache(); cache != nil {
		if r.moduleWatcherFactory != nil {
			ctrlBuilder = r.moduleWatcherFactory(cache, ctrlBuilder)
		}
		ctrlBuilder = r.attachPoolDependencyWatcher(cache, ctrlBuilder)
	}

	return ctrlBuilder.Complete(r)
}

func (r *Reconciler) attachModuleWatcher(b controllerbuilder.Builder, c cache.Cache) controllerbuilder.Builder {
	moduleConfig := &unstructured.Unstructured{}
	moduleConfig.SetGroupVersionKind(moduleconfig.ModuleConfigGVK)
	handlerFunc := handler.TypedEnqueueRequestsFromMapFunc(r.mapModuleConfig)
	return b.WatchesRawSource(source.Kind(c, moduleConfig, handlerFunc))
}

func (r *Reconciler) mapModuleConfig(ctx context.Context, _ *unstructured.Unstructured) []reconcile.Request {
	if r.store != nil && !r.store.Current().Enabled {
		return nil
	}
	return r.requeueAllPools(ctx)
}

func (r *Reconciler) attachPoolDependencyWatcher(c cache.Cache, b controllerbuilder.Builder) controllerbuilder.Builder {
	dev := &v1alpha1.GPUDevice{}
	b = b.WatchesRawSource(source.Kind(c, dev, handler.TypedEnqueueRequestsFromMapFunc(r.mapDevice), devicePredicates()))

	pod := &corev1.Pod{}
	b = b.WatchesRawSource(source.Kind(c, pod, handler.TypedEnqueueRequestsFromMapFunc(r.mapValidatorPod), validatorPodPredicates()))

	return b
}

func (r *Reconciler) mapDevice(ctx context.Context, dev *v1alpha1.GPUDevice) []reconcile.Request {
	if dev == nil {
		return nil
	}

	targetPools := map[string]struct{}{}
	reqSet := map[types.NamespacedName]struct{}{}

	if ref := dev.Status.PoolRef; ref != nil {
		if ref.Name != "" {
			if ref.Namespace != "" {
				reqSet[types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}] = struct{}{}
			} else {
				targetPools[ref.Name] = struct{}{}
			}
		}
	}

	if ann := dev.Annotations[assignmentAnnotation]; ann != "" {
		targetPools[ann] = struct{}{}
	}

	if len(targetPools) == 0 || r.client == nil {
		if len(reqSet) == 0 {
			return nil
		}

		reqs := make([]reconcile.Request, 0, len(reqSet))
		for nn := range reqSet {
			reqs = append(reqs, reconcile.Request{NamespacedName: nn})
		}
		return reqs
	}

	for poolName := range targetPools {
		list := &v1alpha1.GPUPoolList{}
		if err := r.client.List(ctx, list, client.MatchingFields{indexer.GPUPoolNameField: poolName}); err != nil {
			if r.log.GetSink() != nil {
				r.log.Error(err, "list GPUPool by name to map device event", "device", dev.Name, "pool", poolName)
			}
			continue
		}
		for i := range list.Items {
			pool := list.Items[i]
			reqSet[types.NamespacedName{Namespace: pool.Namespace, Name: pool.Name}] = struct{}{}
		}
	}

	if len(reqSet) == 0 {
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(reqSet))
	for nn := range reqSet {
		reqs = append(reqs, reconcile.Request{NamespacedName: nn})
	}
	return reqs
}

func (r *Reconciler) mapValidatorPod(ctx context.Context, pod *corev1.Pod) []reconcile.Request {
	if pod == nil || pod.Labels == nil {
		return nil
	}
	if pod.Labels["app"] != "nvidia-operator-validator" {
		return nil
	}
	poolName := strings.TrimSpace(pod.Labels["pool"])
	if poolName == "" || r.client == nil {
		return nil
	}

	list := &v1alpha1.GPUPoolList{}
	if err := r.client.List(ctx, list, client.MatchingFields{indexer.GPUPoolNameField: poolName}); err != nil {
		if r.log.GetSink() != nil {
			r.log.Error(err, "list GPUPool by name to map validator pod event", "pod", pod.Name, "pool", poolName)
		}
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		pool := list.Items[i]
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: pool.Namespace, Name: pool.Name}})
	}
	return reqs
}

func devicePredicates() predicate.TypedPredicate[*v1alpha1.GPUDevice] {
	return predicate.TypedFuncs[*v1alpha1.GPUDevice]{
		CreateFunc: func(e event.TypedCreateEvent[*v1alpha1.GPUDevice]) bool {
			dev := e.Object
			return dev != nil && (dev.Annotations[assignmentAnnotation] != "" || dev.Status.PoolRef != nil)
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha1.GPUDevice]) bool {
			oldDev := e.ObjectOld
			newDev := e.ObjectNew
			if oldDev == nil || newDev == nil {
				return true
			}
			return deviceChanged(oldDev, newDev)
		},
		DeleteFunc:  func(event.TypedDeleteEvent[*v1alpha1.GPUDevice]) bool { return true },
		GenericFunc: func(event.TypedGenericEvent[*v1alpha1.GPUDevice]) bool { return false },
	}
}

func validatorPodPredicates() predicate.TypedPredicate[*corev1.Pod] {
	return predicate.TypedFuncs[*corev1.Pod]{
		CreateFunc: func(e event.TypedCreateEvent[*corev1.Pod]) bool {
			return isValidatorPoolPod(e.Object)
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Pod]) bool {
			oldPod, newPod := e.ObjectOld, e.ObjectNew
			if oldPod == nil || newPod == nil {
				return true
			}
			if !isValidatorPoolPod(newPod) {
				return false
			}
			if !isValidatorPoolPod(oldPod) {
				return true
			}
			if oldPod.Spec.NodeName != newPod.Spec.NodeName {
				return true
			}
			return podReady(oldPod) != podReady(newPod)
		},
		DeleteFunc:  func(e event.TypedDeleteEvent[*corev1.Pod]) bool { return isValidatorPoolPod(e.Object) },
		GenericFunc: func(event.TypedGenericEvent[*corev1.Pod]) bool { return false },
	}
}

func isValidatorPoolPod(pod *corev1.Pod) bool {
	if pod == nil || pod.Labels == nil {
		return false
	}
	if pod.Labels["app"] != "nvidia-operator-validator" {
		return false
	}
	return strings.TrimSpace(pod.Labels["pool"]) != ""
}

func podReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func deviceChanged(oldDev, newDev *v1alpha1.GPUDevice) bool {
	if oldDev.Annotations[assignmentAnnotation] != newDev.Annotations[assignmentAnnotation] {
		return true
	}
	if oldDev.Status.State != newDev.Status.State || oldDev.Status.NodeName != newDev.Status.NodeName {
		return true
	}
	if oldDev.Status.Hardware.UUID != newDev.Status.Hardware.UUID {
		return true
	}
	if !equality.Semantic.DeepEqual(oldDev.Status.Hardware.MIG, newDev.Status.Hardware.MIG) {
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
	log := crlog.FromContext(ctx).WithValues("pool", req.Name)
	ctx = logr.NewContext(ctx, log)

	pool := &v1alpha1.GPUPool{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, pool); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(2).Info("GPUPool removed")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	resource := reconciler.NewResource(pool, r.client)

	rec := reconciler.NewBase(r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, handler contracts.PoolHandler) (contracts.Result, error) {
		return handler.HandlePool(ctx, pool)
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		return resource.PatchStatus(ctx)
	})

	res, err := rec.Reconcile(ctx)
	if err != nil {
		log.Error(err, "handler chain failed")
		return ctrl.Result{}, err
	}

	return ctrl.Result{
		Requeue:      res.Requeue,
		RequeueAfter: res.RequeueAfter,
	}, nil
}

func (r *Reconciler) requeueAllPools(ctx context.Context) []reconcile.Request {
	list := &v1alpha1.GPUPoolList{}
	if err := r.client.List(ctx, list); err != nil {
		if r.log.GetSink() != nil {
			r.log.Error(err, "list GPUPool to resync after module config change")
		}
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for _, pool := range list.Items {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: pool.Namespace,
				Name:      pool.Name,
			},
		})
	}
	return reqs
}
