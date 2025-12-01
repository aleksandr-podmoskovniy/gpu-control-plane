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
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
)

const (
	localPoolResourcePrefix   = "gpu.deckhouse.io/"
	clusterPoolResourcePrefix = "cluster.gpu.deckhouse.io/"
	poolAnnotation            = "gpu.deckhouse.io/pool"
)

type poolRequest struct {
	name      string
	keyPrefix string
	isCluster bool
}

var jsonMarshal = json.Marshal

type podMutator struct {
	log     logr.Logger
	decoder cradmission.Decoder
	store   *config.ModuleConfigStore
	client  client.Client
}

func newPodMutator(log logr.Logger, decoder cradmission.Decoder, store *config.ModuleConfigStore, c client.Client) *podMutator {
	return &podMutator{
		log:     log.WithName("pod-mutator"),
		decoder: decoder,
		store:   store,
		client:  c,
	}
}

func (m *podMutator) Handle(ctx context.Context, req cradmission.Request) cradmission.Response {
	pod := &corev1.Pod{}
	switch {
	case len(req.Object.Raw) > 0:
		if err := json.Unmarshal(req.Object.Raw, pod); err != nil {
			return cradmission.Errored(422, err)
		}
	case req.Object.Object != nil:
		if p, ok := req.Object.Object.(*corev1.Pod); ok {
			pod = p
		} else {
			return cradmission.Errored(422, fmt.Errorf("request object is not a Pod"))
		}
	default:
		return cradmission.Denied("empty pod admission request")
	}

	pools := collectPools(pod)
	if len(pools) == 0 {
		return cradmission.Allowed("no gpu pool requested")
	}
	if len(pools) > 1 {
		names := make([]string, 0, len(pools))
		for _, p := range pools {
			names = append(names, p.keyPrefix+p.name)
		}
		return cradmission.Denied(fmt.Sprintf("multiple GPU pools requested: %v", names))
	}

	original := req.Object.Raw
	if len(original) == 0 {
		// fallback to current pod snapshot when raw body is missing
		if raw, err := json.Marshal(pod); err == nil {
			original = raw
		}
	}

	var poolRef poolRequest
	for _, p := range pools {
		poolRef = p
	}
	poolKey := poolLabelKey(poolRef)
	poolObj, err := m.resolvePool(ctx, poolRef, pod.Namespace)
	if err != nil {
		return cradmission.Denied(err.Error())
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[poolAnnotation] = poolRef.keyPrefix + poolRef.name

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
	}
	strategy, topologyKey := m.poolScheduling(poolObj)
	if strings.EqualFold(strategy, string(v1alpha1.GPUPoolSchedulingSpread)) {
		if err := ensureSpreadConstraint(pod, poolKey, poolRef.name, topologyKey); err != nil {
			return cradmission.Denied(err.Error())
		}
	}
	ensureCustomTolerations(pod, m.store)

	mutated, err := jsonMarshal(pod)
	if err != nil {
		return cradmission.Errored(500, fmt.Errorf("marshal mutated pod: %w", err))
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
					pools["local:"+pool] = poolRequest{name: pool, keyPrefix: localPoolResourcePrefix, isCluster: false}
				}
			case strings.HasPrefix(name, clusterPoolResourcePrefix):
				pool := strings.TrimPrefix(name, clusterPoolResourcePrefix)
				if pool != "" {
					pools["cluster:"+pool] = poolRequest{name: pool, keyPrefix: clusterPoolResourcePrefix, isCluster: true}
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

func poolLabelKey(pool poolRequest) string {
	return pool.keyPrefix + pool.name
}

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

func (m *podMutator) resolvePool(ctx context.Context, req poolRequest, ns string) (*v1alpha1.GPUPool, error) {
	if m.client == nil {
		return nil, fmt.Errorf("GPUPool %q: webhook client is not configured", req.name)
	}
	if ns == "" && !req.isCluster {
		return nil, fmt.Errorf("GPUPool %q: pod namespace is empty", req.name)
	}
	if req.isCluster {
		cluster := &v1alpha1.ClusterGPUPool{}
		if err := m.client.Get(ctx, client.ObjectKey{Name: req.name}, cluster); err == nil {
			return &v1alpha1.GPUPool{
				ObjectMeta: metav1.ObjectMeta{Name: cluster.Name},
				Spec:       cluster.Spec,
			}, nil
		}
		return nil, fmt.Errorf("ClusterGPUPool %q not found", req.name)
	}

	pool := &v1alpha1.GPUPool{}
	if err := m.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: req.name}, pool); err == nil {
		return pool, nil
	}
	return nil, fmt.Errorf("GPUPool %q not found in namespace %q", req.name, ns)
}
