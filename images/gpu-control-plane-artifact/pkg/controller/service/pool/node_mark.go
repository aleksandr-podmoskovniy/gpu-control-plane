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

package pool

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
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

	poolKey := PoolLabelKey(pool)
	altPoolKey := AlternatePoolLabelKey(pool)
	taintsEnabled := pool.Spec.Scheduling.TaintsEnabled != nil && *pool.Spec.Scheduling.TaintsEnabled

	nodesWithDevices := make(map[string]int32)

	devices := &v1alpha1.GPUDeviceList{}
	if err := h.client.List(ctx, devices, client.MatchingFields{indexer.GPUDevicePoolRefNameField: pool.Name}); err != nil {
		return contracts.Result{}, err
	}
	for i := range devices.Items {
		dev := &devices.Items[i]
		if IsDeviceIgnored(dev) {
			continue
		}
		if !PoolRefMatchesPool(pool, dev.Status.PoolRef) {
			continue
		}
		nodeName := DeviceNodeName(dev)
		if nodeName == "" {
			continue
		}
		nodesWithDevices[nodeName]++
	}

	nodesToSync := make(map[string]struct{}, len(nodesWithDevices))
	for nodeName := range nodesWithDevices {
		nodesToSync[nodeName] = struct{}{}
	}

	poolValue := PoolValueFromKey(poolKey)
	nodeList := &corev1.NodeList{}
	if err := h.client.List(ctx, nodeList, client.MatchingLabels{poolKey: poolValue}); err != nil {
		return contracts.Result{}, err
	}
	for i := range nodeList.Items {
		nodesToSync[nodeList.Items[i].Name] = struct{}{}
	}

	taintedNodes := &corev1.NodeList{}
	if err := h.client.List(ctx, taintedNodes, client.MatchingFields{indexer.NodeTaintKeyField: poolKey}); err != nil {
		return contracts.Result{}, err
	}
	for i := range taintedNodes.Items {
		nodesToSync[taintedNodes.Items[i].Name] = struct{}{}
	}

	if altPoolKey != "" {
		altList := &corev1.NodeList{}
		if err := h.client.List(ctx, altList, client.MatchingLabels{altPoolKey: poolValue}); err != nil {
			return contracts.Result{}, err
		}
		for i := range altList.Items {
			nodesToSync[altList.Items[i].Name] = struct{}{}
		}

		altTainted := &corev1.NodeList{}
		if err := h.client.List(ctx, altTainted, client.MatchingFields{indexer.NodeTaintKeyField: altPoolKey}); err != nil {
			return contracts.Result{}, err
		}
		for i := range altTainted.Items {
			nodesToSync[altTainted.Items[i].Name] = struct{}{}
		}
	}

	for nodeName := range nodesToSync {
		total := nodesWithDevices[nodeName]
		if err := h.syncNode(ctx, nodeName, poolKey, altPoolKey, total > 0, taintsEnabled); err != nil {
			return contracts.Result{}, err
		}
	}
	return contracts.Result{}, nil
}

func (h *NodeMarkHandler) syncNode(ctx context.Context, nodeName, poolKey, altPoolKey string, hasDevices bool, taintsEnabled bool) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node := &corev1.Node{}
		node, err := commonobject.FetchObject(ctx, client.ObjectKey{Name: nodeName}, h.client, node)
		if err != nil {
			return err
		}
		if node == nil {
			return nil
		}
		original := node.DeepCopy()

		changed := false
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		poolValue := PoolValueFromKey(poolKey)
		if hasDevices {
			if node.Labels[poolKey] != poolValue {
				node.Labels[poolKey] = poolValue
				changed = true
			}
			// Remove legacy label with different prefix for the same pool name.
			if altPoolKey != "" {
				if _, ok := node.Labels[altPoolKey]; ok {
					delete(node.Labels, altPoolKey)
					changed = true
				}
			}
		} else {
			if _, ok := node.Labels[poolKey]; ok {
				delete(node.Labels, poolKey)
				changed = true
			}
			if altPoolKey != "" {
				if _, ok := node.Labels[altPoolKey]; ok {
					delete(node.Labels, altPoolKey)
					changed = true
				}
			}
		}

		// Default taint policy: apply NoSchedule when devices present; when devices are gone, remove the taint.
		// This keeps bootstrap workloads running on GPU nodes even when pools are reconfigured.
		if taintsEnabled {
			var desiredTaints []corev1.Taint
			if hasDevices {
				desiredTaints = []corev1.Taint{{
					Key:    poolKey,
					Value:  poolValue,
					Effect: corev1.TaintEffectNoSchedule,
				}}
			}

			newTaints, taintsChanged := ensureTaints(node.Spec.Taints, desiredTaints, poolKey)
			if taintsChanged {
				node.Spec.Taints = newTaints
				changed = true
			}
			// Ensure taints of the alternate prefix are removed as well.
			if altPoolKey != "" {
				newAlt, altChanged := ensureTaints(node.Spec.Taints, nil, altPoolKey)
				if altChanged {
					node.Spec.Taints = newAlt
					changed = true
				}
			}
		} else {
			newTaints, taintsChanged := ensureTaints(node.Spec.Taints, []corev1.Taint{}, poolKey)
			if taintsChanged {
				node.Spec.Taints = newTaints
				changed = true
			}
			if altPoolKey != "" {
				newAlt, altChanged := ensureTaints(node.Spec.Taints, []corev1.Taint{}, altPoolKey)
				if altChanged {
					node.Spec.Taints = newAlt
					changed = true
				}
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
