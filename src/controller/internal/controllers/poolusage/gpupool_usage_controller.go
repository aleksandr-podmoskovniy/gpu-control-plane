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
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/podlabels"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controllerbuilder"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/reconciler"
)

const (
	gpuPoolUsageControllerName = "gpu-pool-usage-controller"
	cacheSyncTimeoutDuration   = 10 * time.Minute
)

type GPUPoolUsageReconciler struct {
	client client.Client
	log    logr.Logger
	cfg    config.ControllerConfig
	store  *config.ModuleConfigStore

	builders func(ctrl.Manager) controllerbuilder.Builder
}

func NewGPUPoolUsage(log logr.Logger, cfg config.ControllerConfig, store *config.ModuleConfigStore) *GPUPoolUsageReconciler {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	return &GPUPoolUsageReconciler{
		log:      log,
		cfg:      cfg,
		store:    store,
		builders: controllerbuilder.NewManagedBy,
	}
}

func (r *GPUPoolUsageReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	r.client = mgr.GetClient()

	options := controller.Options{
		MaxConcurrentReconciles: r.cfg.Workers,
		RecoverPanic:            ptr.To(true),
		LogConstructor:          logger.NewConstructor(r.log),
		CacheSyncTimeout:        cacheSyncTimeoutDuration,
		NewQueue:                reconciler.NewNamedQueue(reconciler.UsePriorityQueue()),
	}

	ctrlBuilder := r.builders(mgr).
		Named(gpuPoolUsageControllerName).
		For(&v1alpha1.GPUPool{}, builder.WithPredicates(namespacedPoolPredicates())).
		WithOptions(options)

	if c := mgr.GetCache(); c != nil {
		ctrlBuilder = r.attachPodWatcher(c, ctrlBuilder)
	}

	return ctrlBuilder.Complete(r)
}

func (r *GPUPoolUsageReconciler) attachPodWatcher(c cache.Cache, b controllerbuilder.Builder) controllerbuilder.Builder {
	pod := &corev1.Pod{}
	return b.WatchesRawSource(source.Kind(
		c,
		pod,
		handler.TypedEnqueueRequestsFromMapFunc(r.mapPodToPool),
		gpuWorkloadPodPredicates(podlabels.PoolScopeNamespaced),
	))
}

func (r *GPUPoolUsageReconciler) mapPodToPool(_ context.Context, pod *corev1.Pod) []reconcile.Request {
	if pod == nil || pod.Labels == nil {
		return nil
	}
	if pod.Labels[podlabels.PoolScopeKey] != podlabels.PoolScopeNamespaced {
		return nil
	}
	poolName := strings.TrimSpace(pod.Labels[podlabels.PoolNameKey])
	if poolName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: pod.Namespace, Name: poolName}}}
}

func (r *GPUPoolUsageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := crlog.FromContext(ctx).WithValues("pool", req.String())
	ctx = logr.NewContext(ctx, log)

	if r.store != nil && !r.store.Current().Enabled {
		log.V(2).Info("module disabled, skipping pool usage reconciliation")
		return ctrl.Result{}, nil
	}

	pool := &v1alpha1.GPUPool{}
	if err := r.client.Get(ctx, req.NamespacedName, pool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	pods := &corev1.PodList{}
	if err := r.client.List(ctx, pods,
		client.InNamespace(pool.Namespace),
		client.MatchingLabels{
			podlabels.PoolNameKey:  pool.Name,
			podlabels.PoolScopeKey: podlabels.PoolScopeNamespaced,
		}); err != nil {
		return ctrl.Result{}, err
	}

	resourceName := corev1.ResourceName("gpu.deckhouse.io/" + pool.Name)
	var used int64
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !podCountsTowardsUsage(pod) {
			continue
		}
		used += requestedResources(pod, resourceName)
	}

	used32 := clampInt64ToInt32(used)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha1.GPUPool{}
		if err := r.client.Get(ctx, req.NamespacedName, current); err != nil {
			return client.IgnoreNotFound(err)
		}
		original := current.DeepCopy()

		total := current.Status.Capacity.Total
		available := total - used32
		if available < 0 {
			available = 0
		}

		if current.Status.Capacity.Used == used32 && current.Status.Capacity.Available == available {
			return nil
		}

		current.Status.Capacity.Used = used32
		current.Status.Capacity.Available = available
		return r.client.Status().Patch(ctx, current, client.MergeFrom(original))
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
