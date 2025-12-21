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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

// ensureNodeTolerations adds Exists tolerations for taints present on nodes listed in the pool status.
// This allows workloads to schedule onto tainted GPU nodes without manual toleration wiring.
func (d *PodDefaulter) ensureNodeTolerations(ctx context.Context, pod *corev1.Pod, pool *v1alpha1.GPUPool) error {
	if d.client == nil || pool == nil {
		return nil
	}

	taints, err := d.collectPoolNodeTaints(ctx, pool)
	if err != nil {
		return err
	}
	for _, taint := range taints {
		if toleratesTaint(pod.Spec.Tolerations, taint) {
			continue
		}
		op := corev1.TolerationOpEqual
		value := taint.Value
		if taint.Value == "" {
			op = corev1.TolerationOpExists
		}
		pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{
			Key:      taint.Key,
			Operator: op,
			Value:    value,
			Effect:   taint.Effect,
		})
	}
	return nil
}

// collectPoolNodeTaints returns taints from nodes participating in the pool.
func (d *PodDefaulter) collectPoolNodeTaints(ctx context.Context, pool *v1alpha1.GPUPool) ([]corev1.Taint, error) {
	seen := make(map[string]corev1.Taint)

	add := func(node *corev1.Node) {
		for _, t := range node.Spec.Taints {
			key := fmt.Sprintf("%s|%s|%s", t.Key, t.Value, t.Effect)
			seen[key] = t
		}
	}

	prefix := localPoolResourcePrefix
	if pool.Namespace == "" {
		prefix = clusterPoolResourcePrefix
	}
	poolKey := prefix + pool.Name
	nodes := &corev1.NodeList{}
	if err := d.client.List(ctx, nodes, client.MatchingLabels{poolKey: pool.Name}); err != nil {
		return nil, fmt.Errorf("list pool nodes for tolerations: %w", err)
	}
	for i := range nodes.Items {
		add(&nodes.Items[i])
	}

	out := make([]corev1.Taint, 0, len(seen))
	for _, t := range seen {
		out = append(out, t)
	}
	return out, nil
}

// topologyLabelPresent checks whether any pool node has the required topology label key.
func (d *PodDefaulter) topologyLabelPresent(ctx context.Context, poolKey, poolName, topologyKey string) (bool, error) {
	if topologyKey == "" {
		return false, nil
	}
	nodes := &corev1.NodeList{}
	if err := d.client.List(ctx, nodes, client.MatchingLabels{poolKey: poolName}); err != nil {
		return false, fmt.Errorf("list pool nodes for topology spread: %w", err)
	}
	if len(nodes.Items) == 0 {
		// Unknown yet — do not block adding the constraint.
		return true, nil
	}
	for i := range nodes.Items {
		if _, ok := nodes.Items[i].Labels[topologyKey]; ok {
			return true, nil
		}
	}
	return false, nil
}

func toleratesTaint(tolerations []corev1.Toleration, taint corev1.Taint) bool {
	for _, t := range tolerations {
		if t.Key != taint.Key {
			continue
		}
		// empty effect tolerates all; otherwise must match
		if t.Effect != "" && taint.Effect != "" && t.Effect != taint.Effect {
			continue
		}
		// Exists toleration tolerates any value
		if t.Operator == corev1.TolerationOpExists || t.Operator == "" {
			return true
		}
		if t.Operator == corev1.TolerationOpEqual {
			// empty value tolerates any taint value
			if t.Value == "" || t.Value == taint.Value {
				return true
			}
		}
	}
	return false
}
