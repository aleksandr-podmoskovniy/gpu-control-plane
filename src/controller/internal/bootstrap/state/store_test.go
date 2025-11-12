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

package state

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestClient(t *testing.T, objs ...runtime.Object) *clientfake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apps scheme: %v", err)
	}
	return clientfake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...)
}

func TestStoreEnsureCreatesConfigMapWithOwner(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "controller",
			Namespace: "d8-gpu-control-plane",
			UID:       "uid-1",
		},
	}
	client := newTestClient(t, deploy).Build()
	store := NewStore(client, client, "d8-gpu-control-plane", "gpu-state", types.NamespacedName{Name: "controller", Namespace: "d8-gpu-control-plane"})

	if err := store.Ensure(context.Background()); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}

	cm := &corev1.ConfigMap{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "gpu-state", Namespace: "d8-gpu-control-plane"}, cm); err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if len(cm.OwnerReferences) != 1 || cm.OwnerReferences[0].Name != "controller" {
		t.Fatalf("owner reference not set: %+v", cm.OwnerReferences)
	}
}

func TestStoreEnsurePropagatesGetError(t *testing.T) {
	base := newTestClient(t).Build()
	client := &errorClient{Client: base, getErr: errors.New("boom")}
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})
	if err := store.Ensure(context.Background()); err == nil {
		t.Fatal("expected error from Ensure when client.Get fails")
	}
}

func TestStoreEnsureExistingConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "gpu-state", Namespace: "ns"}}
	client := newTestClient(t, cm).Build()
	store := NewStore(client, client, "ns", "gpu-state", types.NamespacedName{})

	if err := store.Ensure(context.Background()); err != nil {
		t.Fatalf("ensure existing configmap: %v", err)
	}
}

func TestStoreEnsureSkipsMissingOwner(t *testing.T) {
	client := newTestClient(t).Build()
	store := NewStore(client, client, "ns", "state", types.NamespacedName{Name: "missing", Namespace: "ns"})

	if err := store.Ensure(context.Background()); err != nil {
		t.Fatalf("ensure should ignore missing owner: %v", err)
	}
}

func TestStoreEnsureOwnerReferenceError(t *testing.T) {
	base := newTestClient(t).Build()
	key := types.NamespacedName{Name: "controller", Namespace: "ns"}
	client := &errorClient{Client: base, getErr: errors.New("owner error"), failKey: &key}
	store := NewStore(client, client, "ns", "state", key)
	if err := store.Ensure(context.Background()); err == nil {
		t.Fatal("expected error when owner lookup fails")
	}
}

func TestStoreUpdateAndDeleteNode(t *testing.T) {
	client := newTestClient(t).Build()
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})
	if err := store.Ensure(context.Background()); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}

	state := NodeState{
		Phase: "Ready",
		Components: map[string]bool{
			"validator": true,
			"dcgm":      false,
		},
	}
	if err := store.UpdateNode(context.Background(), "node-a", state); err != nil {
		t.Fatalf("update node: %v", err)
	}
	// second update should be a no-op
	if err := store.UpdateNode(context.Background(), "node-a", state); err != nil {
		t.Fatalf("idempotent update failed: %v", err)
	}

	cm := &corev1.ConfigMap{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "state", Namespace: "ns"}, cm); err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	payload, ok := cm.Data["node-a.yaml"]
	if !ok {
		t.Fatalf("node state entry missing: %v", cm.Data)
	}
	if !contains(payload, "phase: Ready") || !contains(payload, "validator: true") {
		t.Fatalf("unexpected payload: %s", payload)
	}

	if err := store.DeleteNode(context.Background(), "node-a"); err != nil {
		t.Fatalf("delete node: %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "state", Namespace: "ns"}, cm); err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if _, ok := cm.Data["node-a.yaml"]; ok {
		t.Fatalf("expected node entry removed, data=%v", cm.Data)
	}
}

func TestStoreUpdateNodeWithCorruptPayload(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "state", Namespace: "ns"},
		Data:       map[string]string{"node-a.yaml": "phase: ["},
	}
	client := newTestClient(t, cm).Build()
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})
	state := NodeState{Phase: "Ready"}
	if err := store.UpdateNode(context.Background(), "node-a", state); err == nil {
		t.Fatal("expected error when payload cannot be unmarshalled")
	}
}

func TestStoreUpdateNodeInitialisesComponentsAndData(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "state", Namespace: "ns"}}
	client := newTestClient(t, cm).Build()
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})
	state := NodeState{Phase: "Ready"}
	if err := store.UpdateNode(context.Background(), "node-a", state); err != nil {
		t.Fatalf("update node with nil components failed: %v", err)
	}

	updated := &corev1.ConfigMap{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "state", Namespace: "ns"}, updated); err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if updated.Data == nil {
		t.Fatal("expected data map to be initialised")
	}
	if payload, ok := updated.Data["node-a.yaml"]; !ok || !contains(payload, "components") {
		t.Fatalf("expected components entry, payload=%s", payload)
	}
}

func TestStoreUpdateNodePropagatesGetError(t *testing.T) {
	base := newTestClient(t).Build()
	client := &errorClient{Client: base, getErr: errors.New("boom")}
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})
	if err := store.UpdateNode(context.Background(), "node-a", NodeState{Phase: "Ready"}); err == nil {
		t.Fatal("expected error when Get fails")
	}
}

func TestStoreUpdateNodePropagatesUpdateError(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "state", Namespace: "ns"}}
	base := newTestClient(t, cm).Build()
	client := &errorClient{Client: base, updateErr: errors.New("update fail")}
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})
	if err := store.UpdateNode(context.Background(), "node-a", NodeState{Phase: "Ready"}); err == nil || !strings.Contains(err.Error(), "update fail") {
		t.Fatalf("expected update error, got %v", err)
	}
}

func TestStoreUpdateNodeCreatesConfigMap(t *testing.T) {
	client := newTestClient(t).Build()
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})
	if err := store.UpdateNode(context.Background(), "node-a", NodeState{Phase: "Ready"}); err == nil || !strings.Contains(err.Error(), "retry update") {
		t.Fatalf("expected retry error, got %v", err)
	}
	cm := &corev1.ConfigMap{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "state", Namespace: "ns"}, cm); err != nil {
		t.Fatalf("configmap not created: %v", err)
	}
}

func TestStoreUpdateNodeEnsureError(t *testing.T) {
	base := newTestClient(t).Build()
	owner := types.NamespacedName{Name: "controller", Namespace: "ns"}
	client := &errorClient{
		Client:  base,
		getErr:  errors.New("owner boom"),
		failKey: &owner,
	}
	store := NewStore(client, client, "ns", "state", owner)

	err := store.UpdateNode(context.Background(), "node-a", NodeState{Phase: "Ready"})
	if err == nil || !strings.Contains(err.Error(), "owner boom") {
		t.Fatalf("expected owner error from Ensure, got %v", err)
	}
}

func TestStoreUpdateNodeMarshalError(t *testing.T) {
	client := newTestClient(t).Build()
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})
	if err := store.Ensure(context.Background()); err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	original := marshalNodeState
	marshalNodeState = func(NodeState) ([]byte, error) { return nil, errors.New("marshal fail") }
	defer func() { marshalNodeState = original }()
	if err := store.UpdateNode(context.Background(), "node-a", NodeState{Phase: "Ready"}); err == nil || !strings.Contains(err.Error(), "marshal fail") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}

func TestNormaliseNodeStateInitialisesFields(t *testing.T) {
	orig := currentTime
	currentTime = func() metav1.Time { return metav1.NewTime(time.Unix(123, 0)) }
	defer func() { currentTime = orig }()

	state := NodeState{Phase: "Ready"}
	result := normaliseNodeState(state)
	if result.Components == nil {
		t.Fatal("expected components map initialised")
	}
	if result.UpdatedAt.Time.Unix() != 123 {
		t.Fatalf("unexpected timestamp: %v", result.UpdatedAt)
	}
}

func TestStoreDeleteNodeMissingConfigMap(t *testing.T) {
	client := newTestClient(t).Build()
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})

	if err := store.DeleteNode(context.Background(), "node-a"); err != nil {
		t.Fatalf("delete node should ignore missing configmap: %v", err)
	}
}

func TestStoreDeleteNodeHandlesNilData(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "state", Namespace: "ns"}}
	client := newTestClient(t, cm).Build()
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})

	if err := store.DeleteNode(context.Background(), "node-a"); err != nil {
		t.Fatalf("delete node with nil data: %v", err)
	}
}

func TestStoreDeleteNodeNoEntry(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "state", Namespace: "ns"},
		Data:       map[string]string{"other.yaml": "phase: Ready"},
	}
	client := newTestClient(t, cm).Build()
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})

	if err := store.DeleteNode(context.Background(), "node-a"); err != nil {
		t.Fatalf("delete node without entry: %v", err)
	}
}

func TestStoreDeleteNodePropagatesGetError(t *testing.T) {
	base := newTestClient(t).Build()
	client := &errorClient{Client: base, getErr: errors.New("boom")}
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})
	if err := store.DeleteNode(context.Background(), "node-a"); err == nil {
		t.Fatal("expected error when Get fails")
	}
}

func TestSetOwnerReferenceSkipsEmpty(t *testing.T) {
	client := newTestClient(t).Build()
	store := NewStore(client, client, "ns", "state", types.NamespacedName{})
	cm := &corev1.ConfigMap{}
	if err := store.setOwnerReference(context.Background(), cm); err != nil {
		t.Fatalf("setOwnerReference empty owner: %v", err)
	}
}

func TestSetOwnerReferencePropagatesError(t *testing.T) {
	base := newTestClient(t).Build()
	client := &errorClient{Client: base, getErr: errors.New("boom")}
	store := NewStore(client, client, "ns", "state", types.NamespacedName{Name: "controller", Namespace: "ns"})
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "state"}}
	if err := store.setOwnerReference(context.Background(), cm); err == nil {
		t.Fatal("expected error from setOwnerReference when Get fails")
	}
}

func TestStoreGetReaderPrefersExplicitReader(t *testing.T) {
	client := newTestClient(t).Build()
	reader := newTestClient(t).Build()
	store := NewStore(client, reader, "ns", "state", types.NamespacedName{})
	if got := store.getReader(); got != reader {
		t.Fatalf("expected explicit reader to be used, got %T", got)
	}
}

func TestStoreGetReaderFallsBackToClient(t *testing.T) {
	client := newTestClient(t).Build()
	store := NewStore(client, nil, "ns", "state", types.NamespacedName{})
	if got := store.getReader(); got != client {
		t.Fatalf("expected client fallback when reader is nil, got %T", got)
	}
}

type errorClient struct {
	client.Client
	getErr    error
	updateErr error
	failKey   *types.NamespacedName
}

func (c *errorClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if c.failKey != nil && *c.failKey == key {
		return c.getErr
	}
	if c.getErr != nil && c.failKey == nil {
		return c.getErr
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *errorClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.updateErr != nil {
		return c.updateErr
	}
	return c.Client.Update(ctx, obj, opts...)
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
