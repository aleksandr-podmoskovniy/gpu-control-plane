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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/podlabels"
)

const (
	localPoolResourcePrefix   = "gpu.deckhouse.io/"
	clusterPoolResourcePrefix = "cluster.gpu.deckhouse.io/"
)

type poolRequest struct {
	name      string
	keyPrefix string
}

var jsonMarshal = json.Marshal

type podMutator struct {
	log    logr.Logger
	store  *config.ModuleConfigStore
	client client.Client
}

func newPodMutator(log logr.Logger, store *config.ModuleConfigStore, c client.Client) *podMutator {
	return &podMutator{
		log:    log.WithName("pod-mutator"),
		store:  store,
		client: c,
	}
}

func (m *podMutator) Handle(ctx context.Context, req cradmission.Request) cradmission.Response {
	pod, original, err := decodePodRequest(req)
	if err != nil {
		if err == errEmptyPodAdmissionRequest {
			return cradmission.Denied(err.Error())
		}
		return cradmission.Errored(http.StatusUnprocessableEntity, err)
	}

	poolRef, ok, err := selectSinglePool(pod)
	if err != nil {
		return cradmission.Denied(err.Error())
	}
	if !ok {
		return cradmission.Allowed("no gpu pool requested")
	}

	if err := requireGPUEnabledNamespace(ctx, m.client, pod.Namespace); err != nil {
		return cradmission.Denied(err.Error())
	}

	poolKey := poolLabelKey(poolRef)
	var poolObj *v1alpha1.GPUPool
	if m.client != nil {
		var err error
		poolObj, err = resolvePoolByRequest(ctx, m.client, poolRef, pod.Namespace)
		if err != nil {
			return cradmission.Denied(err.Error())
		}
	}

	if err := ensurePoolUsageLabels(pod, poolRef); err != nil {
		return cradmission.Denied(err.Error())
	}
	if err := ensurePoolNodeSelector(pod, poolKey, poolRef.name); err != nil {
		return cradmission.Denied(err.Error())
	}
	poolTaintsEnabled := m.poolTaintsEnabled(poolObj)
	if poolTaintsEnabled {
		if err := ensurePoolToleration(pod, poolKey, poolRef.name); err != nil {
			return cradmission.Denied(err.Error())
		}
		if err := ensurePoolAffinity(pod, poolKey, poolRef.name); err != nil {
			return cradmission.Denied(err.Error())
		}
		if err := m.ensureNodeTolerations(ctx, pod, poolObj); err != nil {
			return cradmission.Denied(err.Error())
		}
	}
	strategy, topologyKey := m.poolScheduling(poolObj)
	if strings.EqualFold(strategy, string(v1alpha1.GPUPoolSchedulingSpread)) {
		if m.client != nil {
			ok, err := m.topologyLabelPresent(ctx, poolKey, poolRef.name, topologyKey)
			if err != nil {
				return cradmission.Denied(err.Error())
			}
			if ok {
				if err := ensureSpreadConstraint(pod, poolKey, poolRef.name, topologyKey); err != nil {
					return cradmission.Denied(err.Error())
				}
			} else {
				m.log.Info("skip topology spread: no nodes with required label", "pool", poolRef.name, "topologyKey", topologyKey)
			}
		} else {
			if err := ensureSpreadConstraint(pod, poolKey, poolRef.name, topologyKey); err != nil {
				return cradmission.Denied(err.Error())
			}
		}
	}
	ensureCustomTolerations(pod, m.store)

	mutated, err := jsonMarshal(pod)
	if err != nil {
		return cradmission.Errored(http.StatusInternalServerError, fmt.Errorf("marshal mutated pod: %w", err))
	}
	return cradmission.PatchResponseFromRaw(original, mutated)
}

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
	if existing, ok := pod.Labels[podlabels.PoolNameKey]; ok && existing != pool.name {
		return fmt.Errorf("label %q already set to %q", podlabels.PoolNameKey, existing)
	}
	pod.Labels[podlabels.PoolNameKey] = pool.name

	scope := podlabels.PoolScopeNamespaced
	if pool.keyPrefix == clusterPoolResourcePrefix {
		scope = podlabels.PoolScopeCluster
	}
	if existing, ok := pod.Labels[podlabels.PoolScopeKey]; ok && existing != scope {
		return fmt.Errorf("label %q already set to %q", podlabels.PoolScopeKey, existing)
	}
	pod.Labels[podlabels.PoolScopeKey] = scope
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
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{expr}}}}
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

func ensureCustomTolerations(pod *corev1.Pod, store *config.ModuleConfigStore) {
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

func (m *podMutator) GVK() schema.GroupVersionKind {
	return corev1.SchemeGroupVersion.WithKind("Pod")
}

// ensureNodeTolerations adds Exists tolerations for taints present on nodes listed in the pool status.
// This allows workloads to schedule onto tainted GPU nodes without manual toleration wiring.
func (m *podMutator) ensureNodeTolerations(ctx context.Context, pod *corev1.Pod, pool *v1alpha1.GPUPool) error {
	if m.client == nil || pool == nil {
		return nil
	}

	taints, err := m.collectPoolNodeTaints(ctx, pool)
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
func (m *podMutator) collectPoolNodeTaints(ctx context.Context, pool *v1alpha1.GPUPool) ([]corev1.Taint, error) {
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
	if err := m.client.List(ctx, nodes, client.MatchingLabels{poolKey: pool.Name}); err != nil {
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
func (m *podMutator) topologyLabelPresent(ctx context.Context, poolKey, poolName, topologyKey string) (bool, error) {
	if topologyKey == "" {
		return false, nil
	}
	nodes := &corev1.NodeList{}
	if err := m.client.List(ctx, nodes, client.MatchingLabels{poolKey: poolName}); err != nil {
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

// collectPools returns a set of pools referenced in all containers (requests/limits).
func collectPools(pod *corev1.Pod) map[string]poolRequest {
	pools := make(map[string]poolRequest)
	check := func(resources corev1.ResourceList) {
		for res := range resources {
			name := res.String()
			switch {
			case strings.HasPrefix(name, localPoolResourcePrefix):
				pool := strings.TrimPrefix(name, localPoolResourcePrefix)
				if pool != "" {
					pools[localPoolResourcePrefix+pool] = poolRequest{name: pool, keyPrefix: localPoolResourcePrefix}
				}
			case strings.HasPrefix(name, clusterPoolResourcePrefix):
				pool := strings.TrimPrefix(name, clusterPoolResourcePrefix)
				if pool != "" {
					pools[clusterPoolResourcePrefix+pool] = poolRequest{name: pool, keyPrefix: clusterPoolResourcePrefix}
				}
			}
		}
	}

	for _, c := range pod.Spec.Containers {
		check(c.Resources.Limits)
		check(c.Resources.Requests)
	}
	for _, c := range pod.Spec.InitContainers {
		check(c.Resources.Limits)
		check(c.Resources.Requests)
	}
	return pools
}

func poolLabelKey(pool poolRequest) string { return pool.keyPrefix + pool.name }

func (m *podMutator) poolTaintsEnabled(pool *v1alpha1.GPUPool) bool {
	if pool == nil || pool.Spec.Scheduling.TaintsEnabled == nil {
		return true
	}
	return *pool.Spec.Scheduling.TaintsEnabled
}

func (m *podMutator) poolScheduling(pool *v1alpha1.GPUPool) (string, string) {
	var strategy, topologyKey string
	if pool != nil {
		strategy = string(pool.Spec.Scheduling.Strategy)
		topologyKey = pool.Spec.Scheduling.TopologyKey
	}
	if strategy == "" && m.store != nil {
		state := m.store.Current()
		strategy = state.Settings.Scheduling.DefaultStrategy
		topologyKey = state.Settings.Scheduling.TopologyKey
	}
	return strategy, topologyKey
}
