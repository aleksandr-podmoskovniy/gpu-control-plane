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

package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add gpu scheme: %v", err)
	}
	return scheme
}

func readyPod(name, app, node string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: meta.WorkloadsNamespace,
			Labels:    map[string]string{"app": app},
		},
		Spec: corev1.PodSpec{NodeName: node},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{
			Type:   corev1.PodReady,
			Status: corev1.ConditionTrue,
		}}},
	}
}

func TestNewWorkloadStatusHandlerDefaultsNamespace(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), "")
	if handler.namespace != meta.WorkloadsNamespace {
		t.Fatalf("expected namespace %s, got %s", meta.WorkloadsNamespace, handler.namespace)
	}
}

type failingListClient struct {
	client.Client
	err error
}

func (f *failingListClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return f.err
}

func TestWorkloadStatusHandlerSetsReadyCondition(t *testing.T) {
	scheme := newScheme(t)
	node := "worker-a"

	objs := []runtime.Object{
		readyPod("gfd", appGPUFeatureDiscovery, node),
		readyPod("validator", appValidator, node),
		readyPod("dcgm", appDCGM, node),
		readyPod("exporter", appDCGMExporter, node),
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(client)
	handler.clock = func() time.Time { return time.Unix(1700000000, 0) }

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Driver:   v1alpha1.GPUNodeDriver{Version: "535.104.05", ToolkitReady: true},
		},
	}
	inventory.Status.Conditions = []metav1.Condition{{
		Type:   conditionInventoryComplete,
		Status: metav1.ConditionTrue,
	}}

	res, err := handler.HandleNode(context.Background(), inventory)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if res.RequeueAfter != defaultReadyRequeueDelay {
		t.Fatalf("unexpected requeueAfter: %s", res.RequeueAfter)
	}

	cond := findCondition(inventory.Status.Conditions, conditionReadyForPooling)
	if cond == nil {
		t.Fatalf("ready condition not set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected ready condition true, got %s", cond.Status)
	}
	if !inventory.Status.Bootstrap.GFDReady || !inventory.Status.Bootstrap.ToolkitReady {
		t.Fatalf("bootstrap status not updated: %+v", inventory.Status.Bootstrap)
	}
	if !inventory.Status.Monitoring.DCGMReady || inventory.Status.Monitoring.LastHeartbeat == nil {
		t.Fatalf("monitoring status not updated: %+v", inventory.Status.Monitoring)
	}
	if len(inventory.Status.Bootstrap.Workloads) != len(workloadComponents) {
		t.Fatalf("expected %d workload entries, got %d", len(workloadComponents), len(inventory.Status.Bootstrap.Workloads))
	}
	for _, workload := range inventory.Status.Bootstrap.Workloads {
		if !workload.Healthy {
			t.Fatalf("expected workload %s to be healthy", workload.Name)
		}
	}
	if inventory.Status.Bootstrap.Phase != v1alpha1.GPUNodeBootstrapPhaseReady {
		t.Fatalf("expected phase Ready, got %s", inventory.Status.Bootstrap.Phase)
	}
}

func TestWorkloadStatusHandlerReportsComponentPending(t *testing.T) {
	scheme := newScheme(t)
	node := "worker-b"

	// Only GFD ready, validator pending (no pod).
	objs := []runtime.Object{
		readyPod("gfd", appGPUFeatureDiscovery, node),
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(client)
	handler.clock = func() time.Time { return time.Unix(1700000100, 0) }

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Driver:   v1alpha1.GPUNodeDriver{Version: "535.104.05", ToolkitReady: true},
		},
	}
	inventory.Status.Conditions = []metav1.Condition{{
		Type:   conditionInventoryComplete,
		Status: metav1.ConditionTrue,
	}}

	res, err := handler.HandleNode(context.Background(), inventory)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if res.RequeueAfter != defaultNotReadyRequeueDelay {
		t.Fatalf("expected fast requeue when not ready, got %s", res.RequeueAfter)
	}

	cond := findCondition(inventory.Status.Conditions, conditionReadyForPooling)
	if cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("expected ready condition false, got %+v", cond)
	}
	if cond.Reason != reasonDriverNotDetected {
		t.Fatalf("expected reason %s, got %s", reasonDriverNotDetected, cond.Reason)
	}
	if inventory.Status.Bootstrap.Phase != v1alpha1.GPUNodeBootstrapPhaseValidating {
		t.Fatalf("expected phase Validating, got %s", inventory.Status.Bootstrap.Phase)
	}
	foundPending := false
	for _, workload := range inventory.Status.Bootstrap.Workloads {
		if workload.Name == string(meta.ComponentValidator) && !workload.Healthy {
			foundPending = true
			if workload.Message == "" {
				t.Fatalf("expected pending workload message")
			}
		}
	}
	if !foundPending {
		t.Fatalf("expected validator workload to be pending, got %+v", inventory.Status.Bootstrap.Workloads)
	}
}

func TestWorkloadStatusHandlerEnablesGFDAfterValidator(t *testing.T) {
	scheme := newScheme(t)
	node := "worker-c"

	objs := []runtime.Object{
		readyPod("validator", appValidator, node),
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(client)

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
		},
	}
	inventory.Status.Conditions = []metav1.Condition{{
		Type:   conditionInventoryComplete,
		Status: metav1.ConditionTrue,
	}}

	if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if inventory.Status.Bootstrap.Phase != v1alpha1.GPUNodeBootstrapPhaseGFD {
		t.Fatalf("expected phase GFD, got %s", inventory.Status.Bootstrap.Phase)
	}

	cond := findCondition(inventory.Status.Conditions, conditionReadyForPooling)
	if cond == nil || cond.Reason != reasonComponentPending {
		t.Fatalf("expected ready condition pending, got %+v", cond)
	}
}

func TestWorkloadStatusHandlerMarksPhaseFailedOnRegression(t *testing.T) {
	scheme := newScheme(t)
	node := "worker-d"

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(client)

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Bootstrap: v1alpha1.GPUNodeBootstrapStatus{
				Phase: v1alpha1.GPUNodeBootstrapPhaseReady,
			},
		},
	}
	inventory.Status.Conditions = []metav1.Condition{
		{Type: conditionInventoryComplete, Status: metav1.ConditionTrue},
		{Type: conditionReadyForPooling, Status: metav1.ConditionTrue},
	}

	if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if inventory.Status.Bootstrap.Phase != v1alpha1.GPUNodeBootstrapPhaseValidatingFailed {
		t.Fatalf("expected phase ValidatingFailed, got %s", inventory.Status.Bootstrap.Phase)
	}
	cond := findCondition(inventory.Status.Conditions, conditionDriverMissing)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected driver missing condition, got %+v", cond)
	}
}

func TestBootstrapComponentAppsReturnsCopy(t *testing.T) {
	apps := BootstrapComponentApps()
	if len(apps) != len(componentAppNames) {
		t.Fatalf("unexpected apps length: %d", len(apps))
	}
	apps[0] = "modified"
	if BootstrapComponentApps()[0] == "modified" {
		t.Fatal("expected BootstrapComponentApps to return a copy")
	}
}

func TestWorkloadStatusHandlerNameAndClientChecks(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	if handler.Name() != "workload-status" {
		t.Fatalf("unexpected handler name: %s", handler.Name())
	}
	if _, err := handler.HandleNode(context.Background(), &v1alpha1.GPUNodeInventory{}); err == nil {
		t.Fatal("expected error when client is not configured")
	}
}

func TestWorkloadStatusHandlerSkipsEmptyNode(t *testing.T) {
	scheme := newScheme(t)
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(fake.NewClientBuilder().WithScheme(scheme).Build())

	res, err := handler.HandleNode(context.Background(), &v1alpha1.GPUNodeInventory{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected no requeue, got %+v", res)
	}
}

func TestWorkloadStatusHandlerPropagatesListError(t *testing.T) {
	scheme := newScheme(t)
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(&failingListClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		err:    errors.New("boom"),
	})

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "node"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Driver:   v1alpha1.GPUNodeDriver{Version: "1", ToolkitReady: true},
		},
	}

	if _, err := handler.HandleNode(context.Background(), inventory); err == nil {
		t.Fatal("expected aggregated error")
	}
}

func TestIsPodReadyOnNodeDefaultMessage(t *testing.T) {
	scheme := newScheme(t)
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(readyPod("gfd", appGPUFeatureDiscovery, "other")).Build())

	ready, msg, err := handler.isPodReadyOnNode(context.Background(), appGPUFeatureDiscovery, "node-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("expected pod to be reported as not ready")
	}
	if msg != "pod not scheduled on node" {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestIsPodReadyOnNodeSkipsDeletedPods(t *testing.T) {
	scheme := newScheme(t)
	pod := readyPod("validator", appValidator, "node-a")
	ts := metav1.NewTime(time.Unix(1700, 0))
	pod.DeletionTimestamp = &ts
	pod.Finalizers = []string{"cleanup"}

	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(pod).Build())

	ready, msg, err := handler.isPodReadyOnNode(context.Background(), appValidator, "node-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("expected deleted pod to be ignored")
	}
	if msg != "pod not scheduled on node" {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestIsPodReadyOnNodeReportsPendingMessage(t *testing.T) {
	scheme := newScheme(t)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dcgm",
			Namespace: meta.WorkloadsNamespace,
			Labels:    map[string]string{"app": appDCGM},
		},
		Spec: corev1.PodSpec{NodeName: "node-a"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "dcgm",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
				},
			}},
		},
	}

	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(pod).Build())

	ready, msg, err := handler.isPodReadyOnNode(context.Background(), appDCGM, "node-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("expected pod to be not ready")
	}
	if !strings.Contains(msg, "ImagePullBackOff") {
		t.Fatalf("unexpected pending message: %s", msg)
	}
}

func TestMonitoringMessageVariants(t *testing.T) {
	if msg := monitoringMessage(true, componentStatus{}, componentStatus{}); !strings.Contains(msg, "ready") {
		t.Fatalf("unexpected ready message: %s", msg)
	}
	if msg := monitoringMessage(false, componentStatus{Ready: false, Message: "hostengine pending"}, componentStatus{}); !strings.Contains(msg, "hostengine pending") {
		t.Fatalf("unexpected dcgm message: %s", msg)
	}
	if msg := monitoringMessage(false, componentStatus{Ready: true}, componentStatus{Ready: false, Message: "exporter pending"}); !strings.Contains(msg, "exporter pending") {
		t.Fatalf("unexpected exporter message: %s", msg)
	}
	if msg := monitoringMessage(false, componentStatus{Ready: true}, componentStatus{Ready: true}); !strings.Contains(msg, "not ready") {
		t.Fatalf("unexpected fallback message: %s", msg)
	}
}

func TestComponentMessageVariants(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	if msg := handler.componentMessage(true, componentStatus{}, componentStatus{}); !strings.Contains(msg, "ready") {
		t.Fatalf("unexpected ready message: %s", msg)
	}
	if msg := handler.componentMessage(false, componentStatus{Ready: false, Message: "gfd pending"}, componentStatus{}); !strings.Contains(msg, "gfd pending") {
		t.Fatalf("unexpected gfd pending message: %s", msg)
	}
	if msg := handler.componentMessage(false, componentStatus{Ready: true}, componentStatus{Ready: false, Message: "validator pending"}); !strings.Contains(msg, "validator pending") {
		t.Fatalf("unexpected validator pending message: %s", msg)
	}
	if msg := handler.componentMessage(false, componentStatus{Ready: true}, componentStatus{Ready: true}); !strings.Contains(msg, "still running") {
		t.Fatalf("unexpected fallback message: %s", msg)
	}
}

func TestDriverAndToolkitMessages(t *testing.T) {
	if msg := driverMessage(true, componentStatus{}); !strings.Contains(msg, "succeeded") {
		t.Fatalf("unexpected driver message: %s", msg)
	}
	if msg := driverMessage(false, componentStatus{Message: "pod pending"}); !strings.Contains(msg, "pod pending") {
		t.Fatalf("unexpected driver missing message: %s", msg)
	}
	if msg := driverMessage(false, componentStatus{}); !strings.Contains(msg, "not completed yet") {
		t.Fatalf("unexpected default driver message: %s", msg)
	}
	if msg := toolkitMessage(true, componentStatus{}); !strings.Contains(msg, "validation completed") {
		t.Fatalf("unexpected toolkit ready message: %s", msg)
	}
	if msg := toolkitMessage(false, componentStatus{Message: "waiting for toolkit"}); !strings.Contains(msg, "waiting for toolkit") {
		t.Fatalf("unexpected toolkit missing message: %s", msg)
	}
	if msg := toolkitMessage(false, componentStatus{}); !strings.Contains(msg, "still running") {
		t.Fatalf("unexpected default toolkit message: %s", msg)
	}
}

func TestEvaluateReadyForPoolingReasons(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	makeInventory := func() *v1alpha1.GPUNodeInventory {
		return &v1alpha1.GPUNodeInventory{
			Status: v1alpha1.GPUNodeInventoryStatus{
				Hardware: v1alpha1.GPUNodeHardware{Present: true},
			},
		}
	}

	// No devices detected.
	inventory := &v1alpha1.GPUNodeInventory{}
	if ready, reason, _ := handler.evaluateReadyForPooling(inventory, true, true, true, true, true); ready || reason != reasonNoDevices {
		t.Fatalf("expected reason %s, got ready=%t reason=%s", reasonNoDevices, ready, reason)
	}

	// Node disabled by label.
	inventory = makeInventory()
	inventory.Status.Conditions = []metav1.Condition{{Type: conditionManagedDisabled, Status: metav1.ConditionTrue}}
	if ready, reason, _ := handler.evaluateReadyForPooling(inventory, true, true, true, true, true); ready || reason != reasonNodeDisabled {
		t.Fatalf("expected reason %s, got ready=%t reason=%s", reasonNodeDisabled, ready, reason)
	}

	// Inventory incomplete blocks readiness.
	inventory = makeInventory()
	if ready, reason, _ := handler.evaluateReadyForPooling(inventory, false, true, true, true, true); ready || reason != reasonInventoryPending {
		t.Fatalf("expected reason %s for incomplete inventory, got ready=%t reason=%s", reasonInventoryPending, ready, reason)
	}

	// Driver/toolkit/component/monitoring issues.
	check := func(driver, toolkit, component, monitoring bool, expected string) {
		if ready, reason, _ := handler.evaluateReadyForPooling(makeInventory(), true, driver, toolkit, component, monitoring); ready || reason != expected {
			t.Fatalf("expected %s, got ready=%t reason=%s", expected, ready, reason)
		}
	}
	check(false, true, true, true, reasonDriverNotDetected)
	check(true, false, true, true, reasonToolkitNotReady)
	check(true, true, false, true, reasonComponentPending)
	check(true, true, true, false, reasonMonitoringUnhealthy)

	// Happy path.
	if ready, reason, _ := handler.evaluateReadyForPooling(makeInventory(), true, true, true, true, true); !ready || reason != reasonAllChecksPassed {
		t.Fatalf("expected ReadyForPooling, got ready=%t reason=%s", ready, reason)
	}
}

func TestPodPendingMessageVariants(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{
				Type:    corev1.PodReady,
				Status:  corev1.ConditionFalse,
				Message: "not ready",
			}},
		},
	}
	if msg := podPendingMessage(pod); msg != "not ready" {
		t.Fatalf("unexpected ready condition message: %s", msg)
	}

	pod = &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "c",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
				},
			}},
		},
	}
	if msg := podPendingMessage(pod); !strings.Contains(msg, "ImagePullBackOff") {
		t.Fatalf("unexpected waiting message: %s", msg)
	}

	pod = &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "c",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 137},
				},
			}},
		},
	}
	if msg := podPendingMessage(pod); !strings.Contains(msg, "Error") {
		t.Fatalf("unexpected terminated message: %s", msg)
	}

	pod = &corev1.Pod{}
	if msg := podPendingMessage(pod); msg != "pod not ready" {
		t.Fatalf("unexpected fallback message: %s", msg)
	}
}

func TestPodReadyHelper(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
	if !podReady(pod) {
		t.Fatal("expected pod to be ready")
	}
	pod = &corev1.Pod{}
	if podReady(pod) {
		t.Fatal("expected pod without Ready condition to be not ready")
	}
}

func TestUpdateBootstrapStatusClearsWorkloads(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	inventory := &v1alpha1.GPUNodeInventory{}
	inventory.Status.Bootstrap.Workloads = []v1alpha1.GPUNodeBootstrapWorkloadStatus{{Name: "validator", Healthy: true}}
	before := inventory.Status.Bootstrap.Workloads
	handler.clock = func() time.Time { return time.Unix(0, 0) }
	handler.updateBootstrapStatus(inventory, true, true, true, nil)
	if inventory.Status.Bootstrap.Workloads != nil {
		t.Fatalf("expected workloads cleared, got %#v", inventory.Status.Bootstrap.Workloads)
	}
	if inventory.Status.Bootstrap.LastRun == nil {
		t.Fatalf("expected lastRun to be set")
	}
	if len(before) == 0 {
		t.Fatal("expected initial workloads slice to be non-empty")
	}
}

func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

func TestDetermineBootstrapPhase(t *testing.T) {
	inventory := &v1alpha1.GPUNodeInventory{}
	tests := []struct {
		name              string
		inv               *v1alpha1.GPUNodeInventory
		inventoryComplete bool
		validator         bool
		gfd               bool
		monitor           bool
		expected          v1alpha1.GPUNodeBootstrapPhase
	}{
		{name: "disabled", inv: &v1alpha1.GPUNodeInventory{Status: v1alpha1.GPUNodeInventoryStatus{Conditions: []metav1.Condition{{Type: conditionManagedDisabled, Status: metav1.ConditionTrue}}}}, inventoryComplete: true, validator: true, gfd: true, monitor: true, expected: v1alpha1.GPUNodeBootstrapPhaseDisabled},
		{name: "inventory-incomplete", inv: inventory, inventoryComplete: false, validator: true, gfd: true, monitor: true, expected: v1alpha1.GPUNodeBootstrapPhaseValidating},
		{name: "validator-pending", inv: inventory, inventoryComplete: true, validator: false, gfd: true, monitor: true, expected: v1alpha1.GPUNodeBootstrapPhaseValidating},
		{name: "gfd-pending", inv: inventory, inventoryComplete: true, validator: true, gfd: false, monitor: true, expected: v1alpha1.GPUNodeBootstrapPhaseGFD},
		{name: "monitoring", inv: inventory, inventoryComplete: true, validator: true, gfd: true, monitor: false, expected: v1alpha1.GPUNodeBootstrapPhaseMonitoring},
		{name: "ready", inv: inventory, inventoryComplete: true, validator: true, gfd: true, monitor: true, expected: v1alpha1.GPUNodeBootstrapPhaseReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phase := determineBootstrapPhase(tt.inv, tt.inventoryComplete, tt.validator, tt.gfd, tt.monitor)
			if phase != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, phase)
			}
		})
	}
}
