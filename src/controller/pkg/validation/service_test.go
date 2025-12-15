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

package validation

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidatorStatusUsesNodeScopedPods(t *testing.T) {
	scheme := clientgoscheme.Scheme
	cfg := applyDefaults(Config{})

	pods := []client.Object{
		newReadyPod("validator-node1", "node-1", moduleLabelValue, cfg.ValidatorApp, corev1.ConditionTrue),
		newReadyPod("gfd-node2", "node-2", moduleLabelValue, cfg.GFDApp, corev1.ConditionTrue), // should be ignored (other node)
		newReadyPod("dcgm-node1", "node-1", moduleLabelValue, cfg.DCGMApp, corev1.ConditionTrue),
		newReadyPod("exporter-node1", "node-1", moduleLabelValue, cfg.DCGMExporterApp, corev1.ConditionTrue),
		newReadyPod("other-module", "node-1", "other", cfg.GFDApp, corev1.ConditionTrue),
	}

	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&corev1.Pod{}, nodeNameField, indexPodByNodeName).
		WithObjects(pods...).
		Build()

	validator := NewValidator(client, Config{})

	status, err := validator.Status(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.GFDReady {
		t.Fatal("expected GFDReady to be false for pods on another node")
	}
	if !status.DriverReady || !status.ToolkitReady || !status.MonitoringReady {
		t.Fatalf("expected validator, toolkit and monitoring to be ready, got %+v", status)
	}
	if status.Message == "" {
		t.Fatal("expected status message when workloads are not fully ready")
	}
}

func TestValidatorStatusReadyWhenAllComponentsPresent(t *testing.T) {
	scheme := clientgoscheme.Scheme
	cfg := applyDefaults(Config{})
	pods := []client.Object{
		newReadyPod("validator-node1", "node-1", moduleLabelValue, cfg.ValidatorApp, corev1.ConditionTrue),
		newReadyPod("gfd-node1", "node-1", moduleLabelValue, cfg.GFDApp, corev1.ConditionTrue),
		newReadyPod("dcgm-node1", "node-1", moduleLabelValue, cfg.DCGMApp, corev1.ConditionTrue),
		newReadyPod("exporter-node1", "node-1", moduleLabelValue, cfg.DCGMExporterApp, corev1.ConditionTrue),
	}
	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&corev1.Pod{}, nodeNameField, indexPodByNodeName).
		WithObjects(pods...).
		Build()

	validator := NewValidator(client, Config{})
	status, err := validator.Status(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Ready {
		t.Fatalf("expected validator ready, got %+v", status)
	}
	if status.Message != "" {
		t.Fatalf("expected empty message on success, got %q", status.Message)
	}
}

func TestValidatorStatusIgnoresPoolScopedValidatorPods(t *testing.T) {
	scheme := clientgoscheme.Scheme
	cfg := applyDefaults(Config{})

	pods := []client.Object{
		newReadyPodWithLabels(
			"validator-pool-a",
			"node-1",
			moduleLabelValue,
			cfg.ValidatorApp,
			corev1.ConditionTrue,
			map[string]string{"pool": "pool-a"},
		),
	}

	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&corev1.Pod{}, nodeNameField, indexPodByNodeName).
		WithObjects(pods...).
		Build()

	validator := NewValidator(client, Config{})

	status, err := validator.Status(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.DriverReady || status.ToolkitReady {
		t.Fatalf("expected driver/toolkit to be false for pool-scoped validator pods, got %+v", status)
	}
}

func TestValidatorStatusIgnoresNotFoundErrors(t *testing.T) {
	scheme := clientgoscheme.Scheme
	base := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	errClient := &failingClient{Client: base, listErr: apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "missing")}

	validator := &workloadValidator{client: errClient, cfg: applyDefaults(Config{})}

	status, err := validator.Status(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("expected nil error on NotFound, got %v", err)
	}
	if status.Ready {
		t.Fatal("expected workloads to be reported as not ready when list fails with NotFound")
	}
}

func TestValidatorStatusPropagatesListErrors(t *testing.T) {
	scheme := clientgoscheme.Scheme
	base := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	internal := apierrors.NewInternalError(errors.New("boom"))
	errClient := &failingClient{Client: base, listErr: internal}

	validator := &workloadValidator{client: errClient, cfg: applyDefaults(Config{}), allowedApps: allowedApps(applyDefaults(Config{}))}

	if _, err := validator.Status(context.Background(), "node-1"); !apierrors.IsInternalError(err) {
		t.Fatalf("expected internal error, got %v", err)
	}
}

func TestValidatorContextRoundTrip(t *testing.T) {
	status := Result{
		Ready:             true,
		DriverReady:       true,
		ToolkitReady:      true,
		GFDReady:          true,
		DCGMReady:         true,
		DCGMExporterReady: true,
		MonitoringReady:   true,
		Message:           "ok",
	}
	ctx := context.Background()

	if _, ok := StatusFromContext(ctx); ok {
		t.Fatalf("expected status to be absent in empty context")
	}

	ctx = ContextWithStatus(ctx, status)
	got, ok := StatusFromContext(ctx)
	if !ok {
		t.Fatalf("expected stored status in context")
	}
	if got != status {
		t.Fatalf("unexpected status round-trip: %+v", got)
	}
}

func TestAllowedAppsSkipsEmptyValues(t *testing.T) {
	apps := allowedApps(Config{ValidatorApp: "validator", GFDApp: "", DCGMApp: "dcgm", DCGMExporterApp: ""})
	if len(apps) != 2 {
		t.Fatalf("expected only non-empty apps, got %#v", apps)
	}
	if _, ok := apps["validator"]; !ok {
		t.Fatalf("expected validator in allowed apps")
	}
	if _, ok := apps["dcgm"]; !ok {
		t.Fatalf("expected dcgm in allowed apps")
	}
}

func TestFilterPodsHonoursAllowedAppsAndNodeName(t *testing.T) {
	pods := []corev1.Pod{
		{Spec: corev1.PodSpec{NodeName: "node-1"}, ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "a"}}},
		{Spec: corev1.PodSpec{NodeName: "node-2"}, ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "a"}}},
		{Spec: corev1.PodSpec{NodeName: "node-1"}, ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "b"}}},
		{Spec: corev1.PodSpec{NodeName: "node-1"}, ObjectMeta: metav1.ObjectMeta{}}, // missing labels
	}

	allowed := map[string]struct{}{"a": {}}

	filtered := filterPods(pods, "node-1", allowed)
	if len(filtered) != 1 {
		t.Fatalf("expected one pod after filtering, got %d", len(filtered))
	}
	if filtered[0].Spec.NodeName != "node-1" || filtered[0].Labels["app"] != "a" {
		t.Fatalf("unexpected filtered pod: %+v", filtered[0])
	}

	// Allowed apps empty -> no app filtering, only node filter.
	filtered = filterPods(pods, "node-1", map[string]struct{}{})
	if len(filtered) != 3 {
		t.Fatalf("expected three pods on node-1, got %d", len(filtered))
	}

	// Node name empty -> no node filtering.
	filtered = filterPods(pods, "", allowed)
	if len(filtered) != 2 {
		t.Fatalf("expected two pods matching allowed apps on any node, got %d", len(filtered))
	}
}

type failingClient struct {
	client.Client
	listErr error
}

func (c *failingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.listErr != nil {
		return c.listErr
	}
	return c.Client.List(ctx, list, opts...)
}

func newReadyPod(name, node, module, app string, status corev1.ConditionStatus) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "d8-gpu-control-plane",
			Labels: map[string]string{
				"app":          app,
				moduleLabelKey: module,
			},
		},
		Spec: corev1.PodSpec{
			NodeName: node,
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: status,
			}},
		},
	}
}

func newReadyPodWithLabels(name, node, module, app string, status corev1.ConditionStatus, extraLabels map[string]string) *corev1.Pod {
	pod := newReadyPod(name, node, module, app, status)
	if extraLabels == nil {
		return pod
	}
	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	for k, v := range extraLabels {
		pod.Labels[k] = v
	}
	return pod
}

func indexPodByNodeName(obj client.Object) []string {
	if pod, ok := obj.(*corev1.Pod); ok && pod.Spec.NodeName != "" {
		return []string{pod.Spec.NodeName}
	}
	return nil
}
