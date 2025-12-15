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
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestNodeMarkNameAndMissingClient(t *testing.T) {
	h := NewNodeMarkHandler(testr.New(t), nil)
	if h.Name() != "node-mark" {
		t.Fatalf("unexpected handler name: %s", h.Name())
	}
	if _, err := h.HandlePool(context.Background(), &v1alpha1.GPUPool{}); err == nil {
		t.Fatalf("expected error when client is nil")
	}
}

func TestSyncNodeNotFoundIsIgnored(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)
	if err := h.syncNode(context.Background(), "missing", "gpu.deckhouse.io/pool", "cluster.gpu.deckhouse.io/pool", true, true); err != nil {
		t.Fatalf("notfound should be ignored: %v", err)
	}
}

func TestSyncNodeAddsAndRemovesLabelsAndTaints(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)

	if err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool", "cluster.gpu.deckhouse.io/pool", true, true); err != nil {
		t.Fatalf("syncNode add failed: %v", err)
	}

	updated := &corev1.Node{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "node1"}, updated); err != nil {
		t.Fatalf("get node: %v", err)
	}
	if updated.Labels["gpu.deckhouse.io/pool"] != "pool" {
		t.Fatalf("label not set")
	}
	if len(updated.Spec.Taints) != 1 || updated.Spec.Taints[0].Effect != corev1.TaintEffectNoSchedule {
		t.Fatalf("expected NoSchedule taint added")
	}

	// add unrelated taint to ensure it is preserved
	updated.Spec.Taints = append(updated.Spec.Taints, corev1.Taint{Key: "other", Effect: corev1.TaintEffectNoSchedule})
	if err := cl.Update(context.Background(), updated); err != nil {
		t.Fatalf("update: %v", err)
	}

	// remove devices and disable taints to exercise removal path
	if err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool", "cluster.gpu.deckhouse.io/pool", false, false); err != nil {
		t.Fatalf("syncNode remove failed: %v", err)
	}
	final := &corev1.Node{}
	_ = cl.Get(context.Background(), types.NamespacedName{Name: "node1"}, final)
	if _, ok := final.Labels["gpu.deckhouse.io/pool"]; ok {
		t.Fatalf("label should be removed")
	}
	if len(final.Spec.Taints) != 1 || final.Spec.Taints[0].Key != "other" {
		t.Fatalf("expected only other taint to remain, got %+v", final.Spec.Taints)
	}
}

func TestSyncNodeClearsTaintsWhenDevicesGone(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)
	if err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool", "cluster.gpu.deckhouse.io/pool", false, true); err != nil {
		t.Fatalf("syncNode: %v", err)
	}
	out := &corev1.Node{}
	_ = cl.Get(context.Background(), types.NamespacedName{Name: "node1"}, out)
	if len(out.Spec.Taints) != 0 {
		t.Fatalf("expected no taints, got %+v", out.Spec.Taints)
	}
}

func TestHandlePoolTaintsDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-1"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node1",
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	cl := withNodeTaintIndexes(withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme))).WithObjects(node, device).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)
	enabled := false
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{TaintsEnabled: &enabled}},
	}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
	out := &corev1.Node{}
	_ = cl.Get(context.Background(), types.NamespacedName{Name: "node1"}, out)
	if len(out.Spec.Taints) != 0 {
		t.Fatalf("expected no taints when disabled, got %+v", out.Spec.Taints)
	}
	if out.Labels["gpu.deckhouse.io/pool"] != "pool" {
		t.Fatalf("expected pool label set, got %+v", out.Labels)
	}
}

func TestSyncNodeNoChangeShortCircuit(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{"gpu.deckhouse.io/pool": "pool"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{{Key: "gpu.deckhouse.io/pool", Value: "pool", Effect: corev1.TaintEffectNoSchedule}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)
	if err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool", "cluster.gpu.deckhouse.io/pool", true, true); err != nil {
		t.Fatalf("syncNode should succeed even when no changes: %v", err)
	}
}

func TestHandlePoolPropagatesSyncError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-1"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node1",
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	base := withNodeTaintIndexes(withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme))).WithObjects(node, device).Build()
	h := NewNodeMarkHandler(testr.New(t), &failingUpdateClient{Client: base})
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
	}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected error propagated from syncNode")
	}
}

func TestSyncNodeGetError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewNodeMarkHandler(testr.New(t), &failingGetClient{Client: base})
	err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool", "cluster.gpu.deckhouse.io/pool", true, true)
	if err == nil || !apierrors.IsBadRequest(err) {
		t.Fatalf("expected get error, got %v", err)
	}
}

func TestSyncNodeTaintsDisabledWithDevices(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)
	if err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool", "cluster.gpu.deckhouse.io/pool", true, false); err != nil {
		t.Fatalf("syncNode: %v", err)
	}
	out := &corev1.Node{}
	_ = cl.Get(context.Background(), types.NamespacedName{Name: "node1"}, out)
	if len(out.Spec.Taints) != 0 || out.Labels["gpu.deckhouse.io/pool"] != "pool" {
		t.Fatalf("expected label without taints, got %+v %+v", out.Labels, out.Spec.Taints)
	}
}

func TestSyncNodeNoChangesEarlyReturn(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)
	if err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool", "cluster.gpu.deckhouse.io/pool", false, false); err != nil {
		t.Fatalf("syncNode should allow no-op: %v", err)
	}
}

func TestHandlePoolListError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	base := fake.NewClientBuilder().WithScheme(scheme).Build()
	cl := &failingListClient{Client: base}
	h := NewNodeMarkHandler(testr.New(t), cl)
	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
	}
	if _, err := h.HandlePool(context.Background(), pool); err == nil {
		t.Fatalf("expected list error")
	}
}

func TestHandlePoolWithStatuses(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-1"},
		Status: v1alpha1.GPUDeviceStatus{
			NodeName: "node1",
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}
	cl := withNodeTaintIndexes(withPoolDeviceIndexes(fake.NewClientBuilder().WithScheme(scheme))).WithObjects(node, device).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)

	pool := &v1alpha1.GPUPool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool"},
		Spec:       v1alpha1.GPUPoolSpec{Scheduling: v1alpha1.GPUPoolSchedulingSpec{TaintsEnabled: ptrTo(true)}},
	}
	if _, err := h.HandlePool(context.Background(), pool); err != nil {
		t.Fatalf("HandlePool: %v", err)
	}
}

func TestSyncNodeUpdateError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewNodeMarkHandler(testr.New(t), &failingUpdateClient{Client: cl})
	err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool", "cluster.gpu.deckhouse.io/pool", true, true)
	if err == nil || !apierrors.IsConflict(err) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestEnsureTaintsCoversBranches(t *testing.T) {
	current := []corev1.Taint{
		{Key: "gpu.deckhouse.io/pool", Effect: corev1.TaintEffectNoSchedule},
		{Key: "other", Effect: corev1.TaintEffectNoExecute},
	}
	desired := []corev1.Taint{{Key: "gpu.deckhouse.io/pool", Effect: corev1.TaintEffectPreferNoSchedule}}

	out, changed := ensureTaints(current, desired, "gpu.deckhouse.io/pool")
	if !changed {
		t.Fatalf("expected changed true")
	}
	if len(out) != 2 { // other taint + desired
		t.Fatalf("unexpected taints result: %+v", out)
	}
	if out[0].Key != "other" || out[1].Effect != corev1.TaintEffectPreferNoSchedule {
		t.Fatalf("taints not merged as expected: %+v", out)
	}
}

func TestNodeMarkRemovesAltPrefixAndTaints(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
			Labels: map[string]string{
				"cluster.gpu.deckhouse.io/pool": "pool",
				"gpu.deckhouse.io/pool":         "pool",
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "cluster.gpu.deckhouse.io/pool", Value: "pool", Effect: corev1.TaintEffectNoSchedule},
				{Key: "gpu.deckhouse.io/pool", Value: "pool", Effect: corev1.TaintEffectNoSchedule},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)

	if err := h.syncNode(context.Background(), "node1", "cluster.gpu.deckhouse.io/pool", "gpu.deckhouse.io/pool", true, true); err != nil {
		t.Fatalf("syncNode: %v", err)
	}

	updated := &corev1.Node{}
	_ = cl.Get(context.Background(), client.ObjectKey{Name: "node1"}, updated)
	if _, ok := updated.Labels["gpu.deckhouse.io/pool"]; ok {
		t.Fatalf("alt prefix label should be removed")
	}
	for _, tnt := range updated.Spec.Taints {
		if tnt.Key == "gpu.deckhouse.io/pool" {
			t.Fatalf("alt prefix taint should be removed")
		}
	}

	// when devices gone and taintsEnabled true, both pool taints are removed
	if err := h.syncNode(context.Background(), "node1", "cluster.gpu.deckhouse.io/pool", "gpu.deckhouse.io/pool", false, true); err != nil {
		t.Fatalf("syncNode remove path: %v", err)
	}
	_ = cl.Get(context.Background(), client.ObjectKey{Name: "node1"}, updated)
	if _, ok := updated.Labels["cluster.gpu.deckhouse.io/pool"]; ok {
		t.Fatalf("labels should be cleared when no devices")
	}
	if len(updated.Spec.Taints) != 0 {
		t.Fatalf("expected pool taints removed, got %v", updated.Spec.Taints)
	}
}

func TestSyncNodeClearsLabelWhenTaintsDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{"gpu.deckhouse.io/pool": "pool"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{{Key: "gpu.deckhouse.io/pool", Value: "pool", Effect: corev1.TaintEffectNoSchedule}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)
	if err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool", "cluster.gpu.deckhouse.io/pool", false, false); err != nil {
		t.Fatalf("syncNode remove path: %v", err)
	}
	updated := &corev1.Node{}
	_ = cl.Get(context.Background(), client.ObjectKey{Name: "node1"}, updated)
	if len(updated.Spec.Taints) != 0 {
		t.Fatalf("expected taints cleared, got %v", updated.Spec.Taints)
	}
	if _, ok := updated.Labels["gpu.deckhouse.io/pool"]; ok {
		t.Fatalf("label should be removed")
	}
}

func TestSyncNodeWithoutAltPrefix(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)
	if err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool", "", true, true); err != nil {
		t.Fatalf("syncNode: %v", err)
	}
	updated := &corev1.Node{}
	_ = cl.Get(context.Background(), client.ObjectKey{Name: "node1"}, updated)
	if updated.Labels["gpu.deckhouse.io/pool"] != "pool" {
		t.Fatalf("expected label set without alt prefix handling")
	}
}

func TestSyncNodeRemovesLabelsWhenDevicesGone(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{"gpu.deckhouse.io/pool": "pool", "cluster.gpu.deckhouse.io/pool": "pool"},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)

	if err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool", "cluster.gpu.deckhouse.io/pool", false, true); err != nil {
		t.Fatalf("syncNode failed: %v", err)
	}
	updated := &corev1.Node{}
	_ = cl.Get(context.Background(), client.ObjectKey{Name: "node1"}, updated)
	if _, ok := updated.Labels["gpu.deckhouse.io/pool"]; ok {
		t.Fatalf("expected primary label removed")
	}
	if _, ok := updated.Labels["cluster.gpu.deckhouse.io/pool"]; ok {
		t.Fatalf("expected alt label removed")
	}
	if len(updated.Spec.Taints) != 0 {
		t.Fatalf("expected pool taints removed, got %v", updated.Spec.Taints)
	}
}

func TestSyncNodeRemovesAltPrefixWhenTaintsDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{"cluster.gpu.deckhouse.io/pool": "pool"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{{Key: "cluster.gpu.deckhouse.io/pool", Value: "pool", Effect: corev1.TaintEffectNoSchedule}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	h := NewNodeMarkHandler(testr.New(t), cl)
	if err := h.syncNode(context.Background(), "node1", "gpu.deckhouse.io/pool", "cluster.gpu.deckhouse.io/pool", true, false); err != nil {
		t.Fatalf("syncNode failed: %v", err)
	}
	updated := &corev1.Node{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "node1"}, updated); err != nil {
		t.Fatalf("get node: %v", err)
	}
	if _, ok := updated.Labels["cluster.gpu.deckhouse.io/pool"]; ok {
		t.Fatalf("expected alt label removed")
	}
	for _, tnt := range updated.Spec.Taints {
		if tnt.Key == "cluster.gpu.deckhouse.io/pool" {
			t.Fatalf("expected alt taint removed, got %v", updated.Spec.Taints)
		}
	}
}

func TestAlternatePoolLabelKeyVariants(t *testing.T) {
	clusterPool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "p"}, TypeMeta: metav1.TypeMeta{Kind: "ClusterGPUPool"}}
	if alternatePoolLabelKey(clusterPool) != "gpu.deckhouse.io/p" {
		t.Fatalf("unexpected alt for cluster pool: %s", alternatePoolLabelKey(clusterPool))
	}
	nsPool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}}
	if alternatePoolLabelKey(nsPool) != "cluster.gpu.deckhouse.io/p" {
		t.Fatalf("unexpected alt for namespaced pool: %s", alternatePoolLabelKey(nsPool))
	}
	if alternatePoolLabelKey(nil) != "" {
		t.Fatalf("expected empty alt for nil pool")
	}
}

// failingUpdateClient forces Update errors to exercise syncNode update failure.
type failingUpdateClient struct {
	client.Client
}

func (f *failingUpdateClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("nodes").GroupResource(), obj.GetName(), nil)
}

func (f *failingUpdateClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return apierrors.NewConflict(v1alpha1.GroupVersion.WithResource("nodes").GroupResource(), obj.GetName(), nil)
}

type failingGetClient struct {
	client.Client
	notFound bool
}

func (f *failingGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if f.notFound {
		return apierrors.NewNotFound(v1alpha1.GroupVersion.WithResource("gpudevices").GroupResource(), key.Name)
	}
	return apierrors.NewBadRequest("boom")
}

type failingListClient struct {
	client.Client
}

func (f *failingListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return apierrors.NewBadRequest("list failure")
}
