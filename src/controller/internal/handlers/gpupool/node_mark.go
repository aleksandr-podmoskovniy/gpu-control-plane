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
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// NodeMarkHandler adds/removes per-pool labels and taints on nodes hosting pool devices.
type NodeMarkHandler struct {
	log    logr.Logger
	client client.Client
}

func NewNodeMarkHandler(log logr.Logger, c client.Client) *NodeMarkHandler {
	return &NodeMarkHandler{log: log, client: c}
}

func (h *NodeMarkHandler) Name() string {
	return "node-mark"
}

func (h *NodeMarkHandler) HandlePool(ctx context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	if h.client == nil {
		return contracts.Result{}, fmt.Errorf("client is required")
	}

	poolKey := poolLabelKey(pool)
	// Temporarily disable taints to avoid evicting bootstrap workloads during pooling.
	taintsEnabled := false
	nodesWithDevices := make(map[string]int32)
	for _, n := range pool.Status.Nodes {
		nodesWithDevices[n.Name] = n.TotalDevices
	}

	for nodeName, total := range nodesWithDevices {
		if err := h.syncNode(ctx, nodeName, poolKey, total > 0, taintsEnabled); err != nil {
			return contracts.Result{}, err
		}
	}
	return contracts.Result{}, nil
}

func (h *NodeMarkHandler) syncNode(ctx context.Context, nodeName, poolKey string, hasDevices bool, taintsEnabled bool) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node := &corev1.Node{}
		if err := h.client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		original := node.DeepCopy()

		changed := false
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		poolValue := poolValueFromKey(poolKey)
		if hasDevices {
			if node.Labels[poolKey] != poolValue {
				node.Labels[poolKey] = poolValue
				changed = true
			}
		} else if _, ok := node.Labels[poolKey]; ok {
			delete(node.Labels, poolKey)
			changed = true
		}

		// Default taint policy: apply NoSchedule when devices present; when devices gone, apply NoExecute to evict remaining pods of this pool.
		if taintsEnabled {
			var desiredTaints []corev1.Taint
			if hasDevices {
				desiredTaints = []corev1.Taint{{
					Key:    poolKey,
					Value:  poolValue,
					Effect: corev1.TaintEffectNoSchedule,
				}}
			} else {
				desiredTaints = []corev1.Taint{{
					Key:    poolKey,
					Value:  poolValue,
					Effect: corev1.TaintEffectNoExecute,
				}}
			}

			newTaints, taintsChanged := ensureTaints(node.Spec.Taints, desiredTaints, poolKey)
			if taintsChanged {
				node.Spec.Taints = newTaints
				changed = true
			}
		} else {
			newTaints, taintsChanged := ensureTaints(node.Spec.Taints, []corev1.Taint{}, poolKey)
			if taintsChanged {
				node.Spec.Taints = newTaints
				changed = true
			}
		}

		if !changed {
			return nil
		}
		return h.client.Patch(ctx, node, client.MergeFrom(original))
	})
}

func ensureTaints(current []corev1.Taint, desired []corev1.Taint, poolKey string) ([]corev1.Taint, bool) {
	out := make([]corev1.Taint, 0, len(current)+len(desired))
	changed := false
	for _, t := range current {
		if t.Key == poolKey {
			changed = true
			continue
		}
		out = append(out, t)
	}
	if len(desired) > 0 {
		out = append(out, desired...)
		changed = true
	}
	return out, changed
}

func poolLabelKey(pool *v1alpha1.GPUPool) string {
	return fmt.Sprintf("%s/%s", poolPrefix(pool), pool.Name)
}

func poolValueFromKey(key string) string {
	parts := strings.Split(key, "/")
	return parts[len(parts)-1]
}

func poolPrefix(pool *v1alpha1.GPUPool) string {
	if pool != nil && pool.Namespace == "" {
		return "cluster.gpu.deckhouse.io"
	}
	return "gpu.deckhouse.io"
}
