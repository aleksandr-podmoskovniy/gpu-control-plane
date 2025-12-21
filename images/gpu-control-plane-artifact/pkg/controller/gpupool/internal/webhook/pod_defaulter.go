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

package webhook

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

type PodDefaulter struct {
	log    logr.Logger
	store  *moduleconfig.ModuleConfigStore
	client client.Client
}

func NewPodDefaulter(log logr.Logger, store *moduleconfig.ModuleConfigStore, c client.Client) *PodDefaulter {
	return &PodDefaulter{
		log:    log.WithName("pod-webhook"),
		store:  store,
		client: c,
	}
}

func (d *PodDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod but got a %T", obj)
	}

	namespace := effectiveNamespace(ctx, pod.Namespace)

	poolRef, ok, err := selectSinglePool(pod)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	if err := requireGPUEnabledNamespace(ctx, d.client, namespace); err != nil {
		return err
	}

	poolKey := poolLabelKey(poolRef)
	var poolObj *v1alpha1.GPUPool
	if d.client != nil {
		poolObj, err = resolvePoolByRequest(ctx, d.client, poolRef, namespace)
		if err != nil {
			return err
		}
	}

	if err := ensurePoolUsageLabels(pod, poolRef); err != nil {
		return err
	}
	if err := ensurePoolNodeSelector(pod, poolKey, poolRef.name); err != nil {
		return err
	}

	poolTaintsEnabled := d.poolTaintsEnabled(poolObj)
	if poolTaintsEnabled {
		if err := ensurePoolToleration(pod, poolKey, poolRef.name); err != nil {
			return err
		}
		if err := ensurePoolAffinity(pod, poolKey, poolRef.name); err != nil {
			return err
		}
		if err := d.ensureNodeTolerations(ctx, pod, poolObj); err != nil {
			return err
		}
	}

	strategy, topologyKey := d.poolScheduling(poolObj)
	if strings.EqualFold(strategy, string(v1alpha1.GPUPoolSchedulingSpread)) {
		if d.client != nil {
			ok, err := d.topologyLabelPresent(ctx, poolKey, poolRef.name, topologyKey)
			if err != nil {
				return err
			}
			if ok {
				if err := ensureSpreadConstraint(pod, poolKey, poolRef.name, topologyKey); err != nil {
					return err
				}
			} else {
				d.log.Info("skip topology spread: no nodes with required label", "pool", poolRef.name, "topologyKey", topologyKey)
			}
		} else {
			if err := ensureSpreadConstraint(pod, poolKey, poolRef.name, topologyKey); err != nil {
				return err
			}
		}
	}

	ensureCustomTolerations(pod, d.store)
	return nil
}

func (d *PodDefaulter) poolTaintsEnabled(pool *v1alpha1.GPUPool) bool {
	if pool == nil || pool.Spec.Scheduling.TaintsEnabled == nil {
		return true
	}
	return *pool.Spec.Scheduling.TaintsEnabled
}

func (d *PodDefaulter) poolScheduling(pool *v1alpha1.GPUPool) (string, string) {
	var strategy, topologyKey string
	if pool != nil {
		strategy = string(pool.Spec.Scheduling.Strategy)
		topologyKey = pool.Spec.Scheduling.TopologyKey
	}
	if strategy == "" && d.store != nil {
		state := d.store.Current()
		strategy = state.Settings.Scheduling.DefaultStrategy
		topologyKey = state.Settings.Scheduling.TopologyKey
	}
	return strategy, topologyKey
}
