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

package tolerations

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	poolsvc "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool"
)

// BuildCustom builds Exists-tolerations for configured keys, skipping empty and duplicates.
func BuildCustom(keys []string) []corev1.Toleration {
	if len(keys) == 0 {
		return nil
	}
	out := make([]corev1.Toleration, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, corev1.Toleration{
			Key:      k,
			Operator: corev1.TolerationOpExists,
		})
	}
	return out
}

// Merge merges base and extra tolerations, deduping by key/operator/value/effect.
func Merge(base []corev1.Toleration, extra []corev1.Toleration) []corev1.Toleration {
	if len(extra) == 0 {
		return base
	}
	out := make([]corev1.Toleration, 0, len(base)+len(extra))
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, t := range base {
		out = append(out, t)
		seen[t.Key+string(t.Operator)+t.Value+string(t.Effect)] = struct{}{}
	}
	for _, t := range extra {
		key := t.Key + string(t.Operator) + t.Value + string(t.Effect)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	return out
}

// PoolNodeTolerations adds Exists-tolerations for taints present on nodes referenced by the pool status.
func PoolNodeTolerations(ctx context.Context, c client.Client, pool *v1alpha1.GPUPool) []corev1.Toleration {
	if c == nil {
		return nil
	}
	tolerations := make([]corev1.Toleration, 0)
	seen := make(map[string]struct{})

	poolKey := poolsvc.PoolLabelKey(pool)
	nodes := &corev1.NodeList{}
	if err := c.List(ctx, nodes, client.MatchingLabels{poolKey: pool.Name}); err != nil {
		return nil
	}
	for i := range nodes.Items {
		node := &nodes.Items[i]
		for _, t := range node.Spec.Taints {
			key := t.Key + string(t.Effect)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			tolerations = append(tolerations, corev1.Toleration{
				Key:      t.Key,
				Operator: corev1.TolerationOpExists,
				Effect:   t.Effect,
			})
		}
	}
	return tolerations
}
