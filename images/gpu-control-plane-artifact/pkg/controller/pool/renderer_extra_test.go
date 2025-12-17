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
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

type selectiveCreateErrorClient struct {
	client.Client
	err   error
	match func(client.Object) bool
}

func (c *selectiveCreateErrorClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.match != nil && c.match(obj) {
		return c.err
	}
	return c.Client.Create(ctx, obj, opts...)
}

type selectiveDeleteErrorClient struct {
	client.Client
	err   error
	match func(client.Object) bool
}

func (c *selectiveDeleteErrorClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.match != nil && c.match(obj) {
		return c.err
	}
	return c.Client.Delete(ctx, obj, opts...)
}

type selectiveGetErrorClient struct {
	client.Client
	err   error
	match func(client.Object) bool
}

func (c *selectiveGetErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if c.match != nil && c.match(obj) {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func TestNewRendererHandlerUsesEnvDefaultsAndValidatorFallback(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "ns-env")
	t.Setenv("DEFAULT_MIG_STRATEGY", "single")
	t.Setenv("NVIDIA_DEVICE_PLUGIN_IMAGE", "dp:env")
	t.Setenv("NVIDIA_MIG_MANAGER_IMAGE", "mig:env")
	t.Setenv("NVIDIA_VALIDATOR_IMAGE", "val:env")

	h := NewRendererHandler(testr.New(t), nil, RenderConfig{CustomTolerationKeys: []string{"a", "", "a"}})
	if h.cfg.Namespace != "ns-env" || h.cfg.DevicePluginImage != "dp:env" || h.cfg.MIGManagerImage != "mig:env" || h.cfg.DefaultMIGStrategy != "single" {
		t.Fatalf("unexpected env config: %+v", h.cfg)
	}
	if h.cfg.ValidatorImage != "val:env" {
		t.Fatalf("expected validator image from env, got %q", h.cfg.ValidatorImage)
	}
	if len(h.customTolerations) != 1 || h.customTolerations[0].Key != "a" {
		t.Fatalf("expected one custom toleration, got %#v", h.customTolerations)
	}

	t.Setenv("NVIDIA_VALIDATOR_IMAGE", "")
	h = NewRendererHandler(testr.New(t), nil, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag"})
	if h.cfg.ValidatorImage != "dp:tag" {
		t.Fatalf("expected validator image to fall back to device-plugin image, got %q", h.cfg.ValidatorImage)
	}

	if got := buildCustomTolerations(nil); got != nil {
		t.Fatalf("expected nil tolerations for empty keys, got %#v", got)
	}

	base := []corev1.Toleration{{Key: "a", Operator: corev1.TolerationOpExists}}
	extra := []corev1.Toleration{{Key: "a", Operator: corev1.TolerationOpExists}}
	merged := mergeTolerations(base, extra)
	if len(merged) != 1 {
		t.Fatalf("expected merge to deduplicate tolerations, got %#v", merged)
	}
}

func TestResolveResourceName(t *testing.T) {
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	if got := resolveResourceName(pool, ""); got != "pool" {
		t.Fatalf("expected pool name fallback, got %q", got)
	}
	if got := resolveResourceName(nil, "gpu.deckhouse.io/pool"); got != "pool" {
		t.Fatalf("expected prefix stripped name, got %q", got)
	}
	if got := resolveResourceName(nil, "a/b/c"); got != "c" {
		t.Fatalf("expected last segment, got %q", got)
	}
	if got := resolveResourceName(nil, "   "); got != "" {
		t.Fatalf("expected empty name, got %q", got)
	}
}

func TestPoolNodeTolerations(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool"}}
	poolKey := poolLabelKey(pool)

	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{poolKey: "pool"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "k1", Effect: corev1.TaintEffectNoSchedule},
				{Key: "k1", Effect: corev1.TaintEffectNoSchedule},
			},
		},
	}
	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node2",
			Labels: map[string]string{poolKey: "pool"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "k1", Effect: corev1.TaintEffectNoSchedule},
				{Key: "k2", Effect: corev1.TaintEffectNoExecute},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node1, node2).Build()

	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
	tols := h.poolNodeTolerations(context.Background(), pool)
	if len(tols) != 2 {
		t.Fatalf("expected 2 deduplicated tolerations, got %#v", tols)
	}
	seen := map[string]struct{}{}
	for _, tol := range tols {
		seen[tol.Key+string(tol.Effect)] = struct{}{}
	}
	if _, ok := seen["k1"+string(corev1.TaintEffectNoSchedule)]; !ok {
		t.Fatalf("expected k1/NoSchedule toleration, got %#v", tols)
	}
	if _, ok := seen["k2"+string(corev1.TaintEffectNoExecute)]; !ok {
		t.Fatalf("expected k2/NoExecute toleration, got %#v", tols)
	}

	if got := (&RendererHandler{}).poolNodeTolerations(context.Background(), pool); got != nil {
		t.Fatalf("expected nil tolerations with nil client, got %#v", got)
	}

	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	h.client = &failingListClient{Client: base}
	if got := h.poolNodeTolerations(context.Background(), pool); got != nil {
		t.Fatalf("expected nil tolerations on list error, got %#v", got)
	}
}

func TestCreateOrUpdateCoversBranches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns", UID: "uid"}}

	ctx := context.Background()

	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}, Data: map[string]string{"k": "v"}}
	if err := h.createOrUpdate(ctx, cm, pool); err != nil {
		t.Fatalf("create cm: %v", err)
	}
	if err := h.createOrUpdate(ctx, cm, pool); err != nil {
		t.Fatalf("cm no-op: %v", err)
	}
	cmChanged := cm.DeepCopy()
	cmChanged.Data["k"] = "changed"
	if err := h.createOrUpdate(ctx, cmChanged, pool); err != nil {
		t.Fatalf("cm update: %v", err)
	}

	ownerless := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "ownerless", Namespace: "ns"}, Data: map[string]string{"k": "v"}}
	ownerlessClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ownerless).Build()
	h.client = ownerlessClient
	if err := h.createOrUpdate(ctx, ownerless, pool); err != nil {
		t.Fatalf("cm owner attach update: %v", err)
	}

	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "ns"}, Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img"}}}}}}
	h.client = cl
	if err := h.createOrUpdate(ctx, ds, pool); err != nil {
		t.Fatalf("create ds: %v", err)
	}
	if err := h.createOrUpdate(ctx, ds, pool); err != nil {
		t.Fatalf("ds no-op: %v", err)
	}
	dsChanged := ds.DeepCopy()
	dsChanged.Spec.Template.Spec.Containers[0].Image = "changed"
	if err := h.createOrUpdate(ctx, dsChanged, pool); err != nil {
		t.Fatalf("ds update: %v", err)
	}

	if err := h.createOrUpdate(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"}}, pool); err == nil {
		t.Fatalf("expected unsupported object type error")
	}

	getErr := apierrors.NewBadRequest("boom")
	h.client = &selectiveGetErrorClient{
		Client: cl,
		err:    getErr,
		match: func(obj client.Object) bool {
			_, ok := obj.(*corev1.ConfigMap)
			return ok
		},
	}
	if err := h.createOrUpdate(ctx, cm, pool); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected configmap get error, got %v", err)
	}
	h.client = &selectiveGetErrorClient{
		Client: cl,
		err:    getErr,
		match: func(obj client.Object) bool {
			_, ok := obj.(*appsv1.DaemonSet)
			return ok
		},
	}
	if err := h.createOrUpdate(ctx, ds, pool); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected daemonset get error, got %v", err)
	}

	otherNamespacePool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ns-a", UID: "uid"}}
	crossNS := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cross", Namespace: "ns-b"}}
	addOwner(crossNS, otherNamespacePool)
	if len(crossNS.OwnerReferences) != 0 {
		t.Fatalf("expected namespaced pool to not own cross-namespace objects")
	}

	clusterPool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "cluster", UID: "uid"}}
	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owned", Namespace: "ns"}}
	addOwner(obj, clusterPool)
	addOwner(obj, clusterPool)
	if len(obj.OwnerReferences) != 1 || obj.OwnerReferences[0].Kind != "ClusterGPUPool" {
		t.Fatalf("expected single ClusterGPUPool owner, got %#v", obj.OwnerReferences)
	}
	if !hasOwner(obj, clusterPool) {
		t.Fatalf("expected hasOwner to detect cluster owner")
	}
}

func TestHandlePoolCoversErrorBranchesAndMIGPaths(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", UID: "uid"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}

	if _, err := (&RendererHandler{}).HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected error when client is nil")
	}

	base := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build()
	h := &RendererHandler{log: testr.New(t), client: base, cfg: RenderConfig{DevicePluginImage: "dp:tag"}}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected error when namespace is not configured")
	}
	h.cfg.Namespace = "ns"
	h.cfg.DevicePluginImage = ""
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected error when device-plugin image is not configured")
	}

	// Unsupported provider is a no-op.
	h.cfg.DevicePluginImage = "dp:tag"
	pool.Spec.Provider = "Other"
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("unexpected provider error: %v", err)
	}
	pool.Spec.Provider = ""

	// poolHasAssignedDevices error should propagate.
	h = NewRendererHandler(testr.New(t), &failingListClient{Client: base}, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
	pool.Status.Capacity.Total = 0
	if _, err := h.HandlePool(context.Background(), pool); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected list error, got %v", err)
	}

	// poolHasAssignedDevices=false should trigger cleanup.
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-device-plugin-pool-config", Namespace: "ns"}}
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "nvidia-device-plugin-pool", Namespace: "ns"}}
	cl := withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(cm, ds).Build()
	h = NewRendererHandler(testr.New(t), cl, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("unexpected cleanup error: %v", err)
	}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(cm), &corev1.ConfigMap{}); err == nil {
		t.Fatalf("expected configmap to be deleted")
	}

	// Validator image missing should fail reconcileValidator after device-plugin reconciliation.
	pool.Status.Capacity.Total = 1
	h = NewRendererHandler(testr.New(t), withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build(), RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag"})
	h.cfg.ValidatorImage = ""
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected validator image error")
	}

	// MIG pool with missing MIG manager image should skip MIG manager reconciliation.
	poolMIG := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "mig", UID: "uid"},
		Spec:       v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb"}},
		Status:     v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}},
	}
	h = NewRendererHandler(testr.New(t), withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build(), RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
	h.cfg.MIGManagerImage = ""
	if _, err := h.HandlePool(context.Background(), poolMIG); err != nil {
		t.Fatalf("unexpected MIG skip error: %v", err)
	}

	// MIG reconciliation error should propagate.
	createErr := apierrors.NewBadRequest("create failed")
	base = withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build()
	h = NewRendererHandler(testr.New(t), &selectiveCreateErrorClient{
		Client: base,
		err:    createErr,
		match: func(obj client.Object) bool {
			return strings.HasPrefix(obj.GetName(), "nvidia-mig-manager-")
		},
	}, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag", MIGManagerImage: "mig:tag"})
	if _, err := h.HandlePool(context.Background(), poolMIG); err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected MIG create error, got %v", err)
	}

	// cleanupMIGResources error should propagate for non-MIG pools.
	deleteErr := errors.New("delete failed")
	base = withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme)).Build()
	h = NewRendererHandler(testr.New(t), &selectiveDeleteErrorClient{
		Client: base,
		err:    deleteErr,
		match: func(obj client.Object) bool {
			return strings.HasPrefix(obj.GetName(), "nvidia-mig-manager-")
		},
	}, RenderConfig{Namespace: "ns", DevicePluginImage: "dp:tag", ValidatorImage: "val:tag"})
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "pool", UID: "uid"}, Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}}, Status: v1alpha1.GPUPoolStatus{Capacity: v1alpha1.GPUPoolCapacityStatus{Total: 1}}}); err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("expected MIG cleanup error, got %v", err)
	}
}
