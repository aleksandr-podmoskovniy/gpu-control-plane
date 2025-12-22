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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

func ensurePoolNodeSelector(pod *corev1.Pod, poolKey, pool string) error {
	if pod.Spec.NodeSelector == nil {
		pod.Spec.NodeSelector = map[string]string{}
	}
	if existing, ok := pod.Spec.NodeSelector[poolKey]; ok && existing != pool {
		return fmt.Errorf("nodeSelector %q already set to %q", poolKey, existing)
	}
	pod.Spec.NodeSelector[poolKey] = pool
	return nil
}

func ensurePoolUsageLabels(pod *corev1.Pod, pool poolRequest) error {
	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	if existing, ok := pod.Labels[poolcommon.PoolNameKey]; ok && existing != pool.name {
		return fmt.Errorf("label %q already set to %q", poolcommon.PoolNameKey, existing)
	}
	pod.Labels[poolcommon.PoolNameKey] = pool.name

	scope := poolcommon.PoolScopeNamespaced
	if pool.keyPrefix == clusterPoolResourcePrefix {
		scope = poolcommon.PoolScopeCluster
	}
	if existing, ok := pod.Labels[poolcommon.PoolScopeKey]; ok && existing != scope {
		return fmt.Errorf("label %q already set to %q", poolcommon.PoolScopeKey, existing)
	}
	pod.Labels[poolcommon.PoolScopeKey] = scope
	return nil
}

func ensurePoolToleration(pod *corev1.Pod, poolKey, pool string) error {
	for i, tol := range pod.Spec.Tolerations {
		if tol.Key != poolKey {
			continue
		}
		// Exists toleration for this key is always compatible.
		if tol.Operator == corev1.TolerationOpExists {
			return nil
		}
		// Normalize missing operator/effect/value.
		if tol.Operator == "" {
			pod.Spec.Tolerations[i].Operator = corev1.TolerationOpEqual
			tol.Operator = corev1.TolerationOpEqual
		}
		if tol.Effect == "" {
			pod.Spec.Tolerations[i].Effect = corev1.TaintEffectNoSchedule
			tol.Effect = corev1.TaintEffectNoSchedule
		}
		if tol.Operator == corev1.TolerationOpEqual {
			if tol.Value == "" {
				pod.Spec.Tolerations[i].Value = pool
				return nil
			}
			if tol.Value == pool && tol.Effect == corev1.TaintEffectNoSchedule {
				return nil
			}
			if tol.Effect != corev1.TaintEffectNoSchedule {
				return fmt.Errorf("toleration %q has unsupported effect %q", poolKey, tol.Effect)
			}
			return fmt.Errorf("toleration %q already set to %q", poolKey, tol.Value)
		}
		return fmt.Errorf("toleration %q has unsupported operator %q", poolKey, tol.Operator)
	}
	pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{
		Key:      poolKey,
		Operator: corev1.TolerationOpEqual,
		Value:    pool,
		Effect:   corev1.TaintEffectNoSchedule,
	})
	return nil
}

func ensurePoolAffinity(pod *corev1.Pod, poolKey, pool string) error {
	if pod.Spec.Affinity == nil {
		pod.Spec.Affinity = &corev1.Affinity{}
	}
	if pod.Spec.Affinity.NodeAffinity == nil {
		pod.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	req := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	expr := corev1.NodeSelectorRequirement{
		Key:      poolKey,
		Operator: corev1.NodeSelectorOpIn,
		Values:   []string{pool},
	}
	if req == nil {
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{expr}}},
		}
		return nil
	}
	for i := range req.NodeSelectorTerms {
		found := false
		for _, me := range req.NodeSelectorTerms[i].MatchExpressions {
			if me.Key != poolKey {
				continue
			}
			found = true
			if me.Operator == corev1.NodeSelectorOpIn {
				for _, v := range me.Values {
					if v == pool {
						// already compatible
						goto nextTerm
					}
				}
			}
			return fmt.Errorf("nodeAffinity already restricts %q differently", poolKey)
		}
		if !found {
			req.NodeSelectorTerms[i].MatchExpressions = append(req.NodeSelectorTerms[i].MatchExpressions, expr)
		}
	nextTerm:
	}
	pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = req
	return nil
}

func ensureCustomTolerations(pod *corev1.Pod, store *moduleconfig.ModuleConfigStore) {
	if store == nil {
		return
	}
	state := store.Current()
	keys := state.Settings.Placement.CustomTolerationKeys
	if len(keys) == 0 {
		return
	}

	for _, key := range keys {
		if hasToleration(pod.Spec.Tolerations, key) {
			continue
		}
		pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{
			Key:      key,
			Operator: corev1.TolerationOpExists,
		})
	}
}

func hasToleration(tols []corev1.Toleration, key string) bool {
	for _, t := range tols {
		if t.Key == key {
			return true
		}
	}
	return false
}

func ensureSpreadConstraint(pod *corev1.Pod, poolKey, pool, topologyKey string) error {
	if topologyKey == "" {
		// without topology key constraint is ineffective; skip
		return nil
	}

	for i := range pod.Spec.TopologySpreadConstraints {
		t := &pod.Spec.TopologySpreadConstraints[i]
		if t.TopologyKey != topologyKey {
			continue
		}
		if t.LabelSelector == nil {
			continue
		}
		if val, ok := t.LabelSelector.MatchLabels[poolKey]; ok {
			if val == pool {
				// already present
				return nil
			}
			return fmt.Errorf("topologySpreadConstraint already sets %q=%q", poolKey, val)
		}
	}

	pod.Spec.TopologySpreadConstraints = append(pod.Spec.TopologySpreadConstraints, corev1.TopologySpreadConstraint{
		MaxSkew:           1,
		TopologyKey:       topologyKey,
		WhenUnsatisfiable: corev1.DoNotSchedule,
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{poolKey: pool},
		},
	})
	return nil
}
