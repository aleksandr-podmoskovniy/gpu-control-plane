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
	"sort"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/podlabels"
)

const (
	localPoolResourcePrefix   = "gpu.deckhouse.io/"
	clusterPoolResourcePrefix = "cluster.gpu.deckhouse.io/"
)

type poolRequest struct {
	name      string
	keyPrefix string
}

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

type PodValidator struct {
	log    logr.Logger
	client client.Client
}

func NewPodValidator(log logr.Logger, c client.Client) *PodValidator {
	return &PodValidator{
		log:    log.WithName("pod-webhook"),
		client: c,
	}
}

func (v *PodValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (cradmission.Warnings, error) {
	return v.validate(ctx, obj)
}

func (v *PodValidator) ValidateUpdate(ctx context.Context, _ runtime.Object, newObj runtime.Object) (cradmission.Warnings, error) {
	return v.validate(ctx, newObj)
}

func (v *PodValidator) ValidateDelete(_ context.Context, _ runtime.Object) (cradmission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

func (v *PodValidator) validate(ctx context.Context, obj runtime.Object) (cradmission.Warnings, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, fmt.Errorf("expected a Pod but got a %T", obj)
	}

	namespace := effectiveNamespace(ctx, pod.Namespace)

	poolRef, ok, err := selectSinglePool(pod)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	if err := requireGPUEnabledNamespace(ctx, v.client, namespace); err != nil {
		return nil, err
	}

	requested := requestedResources(pod, poolRef)
	if requested <= 0 {
		return nil, nil
	}

	poolObj, err := resolvePoolByRequest(ctx, v.client, poolRef, namespace)
	if err != nil {
		return nil, err
	}

	cond := apimeta.FindStatusCondition(poolObj.Status.Conditions, "Configured")
	if cond != nil && cond.Status == metav1.ConditionFalse {
		return nil, fmt.Errorf("GPU pool %s is not configured: %s", poolRef.keyPrefix+poolRef.name, cond.Message)
	}
	if cond != nil && cond.Status == metav1.ConditionTrue {
		total := int64(poolObj.Status.Capacity.Total)
		if total > 0 && requested > total {
			return nil, fmt.Errorf("requested %d units of %s but pool capacity is %d", requested, poolRef.keyPrefix+poolRef.name, total)
		}
	}

	return nil, nil
}

var _ cradmission.CustomDefaulter = (*PodDefaulter)(nil)
var _ cradmission.CustomValidator = (*PodValidator)(nil)

func effectiveNamespace(ctx context.Context, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if req, err := cradmission.RequestFromContext(ctx); err == nil {
		return req.Namespace
	}
	return ""
}

func selectSinglePool(pod *corev1.Pod) (poolRequest, bool, error) {
	pools := collectPools(pod)
	if len(pools) == 0 {
		return poolRequest{}, false, nil
	}
	if len(pools) == 1 {
		for _, p := range pools {
			return p, true, nil
		}
	}

	names := make([]string, 0, len(pools))
	for _, p := range pools {
		names = append(names, p.keyPrefix+p.name)
	}
	sort.Strings(names)
	return poolRequest{}, false, fmt.Errorf("multiple GPU pools requested: %v", names)
}

const gpuEnabledLabelKey = "gpu.deckhouse.io/enabled"

func requireGPUEnabledNamespace(ctx context.Context, c client.Client, namespace string) error {
	if c == nil {
		return nil
	}
	if strings.TrimSpace(namespace) == "" {
		return fmt.Errorf("pod namespace is empty")
	}

	ns := &corev1.Namespace{}
	if err := c.Get(ctx, client.ObjectKey{Name: namespace}, ns); err != nil {
		return fmt.Errorf("namespace %q not found: %v", namespace, err)
	}
	if ns.Labels[gpuEnabledLabelKey] != "true" {
		return fmt.Errorf("namespace %q is not enabled for GPU (label %s=true is required)", namespace, gpuEnabledLabelKey)
	}
	return nil
}

func resolvePoolByRequest(ctx context.Context, c client.Client, req poolRequest, namespace string) (*v1alpha1.GPUPool, error) {
	if c == nil {
		return nil, fmt.Errorf("GPUPool %q: webhook client is not configured", req.name)
	}

	switch req.keyPrefix {
	case clusterPoolResourcePrefix:
		cluster := &v1alpha1.ClusterGPUPool{}
		if err := c.Get(ctx, client.ObjectKey{Name: req.name}, cluster); err == nil {
			return &v1alpha1.GPUPool{
				TypeMeta:   cluster.TypeMeta,
				ObjectMeta: cluster.ObjectMeta,
				Spec:       cluster.Spec,
				Status:     cluster.Status,
			}, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("ClusterGPUPool %q not found", req.name)
	case localPoolResourcePrefix:
		if strings.TrimSpace(namespace) == "" {
			return nil, fmt.Errorf("GPUPool %q: pod namespace is empty", req.name)
		}
		pool := &v1alpha1.GPUPool{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: req.name}, pool); err == nil {
			return pool, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("GPUPool %q not found in namespace %q", req.name, namespace)
	default:
		return nil, fmt.Errorf("unknown pool resource prefix %q", req.keyPrefix)
	}
}

func requestedResources(pod *corev1.Pod, pool poolRequest) int64 {
	name := corev1.ResourceName(pool.keyPrefix + pool.name)
	value := func(req corev1.ResourceRequirements) int64 {
		if q, ok := req.Limits[name]; ok {
			return q.Value()
		}
		if q, ok := req.Requests[name]; ok {
			return q.Value()
		}
		return 0
	}

	var sumContainers int64
	for _, c := range pod.Spec.Containers {
		sumContainers += value(c.Resources)
	}

	var maxInit int64
	for _, c := range pod.Spec.InitContainers {
		if v := value(c.Resources); v > maxInit {
			maxInit = v
		}
	}

	if sumContainers > maxInit {
		return sumContainers
	}
	return maxInit
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

