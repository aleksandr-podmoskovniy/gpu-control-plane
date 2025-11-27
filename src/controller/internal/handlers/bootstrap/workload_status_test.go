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
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("reader error")
}

func TestWorkloadStatusHandlerSetsReadyCondition(t *testing.T) {
	scheme := newScheme(t)
	node := "worker-a"

	gfd := readyPod("gfd", appGPUFeatureDiscovery, node)
	validator := readyPod("validator", appValidator, node)
	dcgm := readyPod("dcgm", appDCGM, node)
	exporter := readyPod("exporter", appDCGMExporter, node)
	exporter.Status.PodIP = "127.0.0.1"

	objs := []runtime.Object{gfd, validator, dcgm, exporter}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(client)
	handler.fetchHeartbeat = func(context.Context, *corev1.Pod) (*metav1.Time, error) {
		ts := metav1.NewTime(time.Unix(1700000000, 0))
		return &ts, nil
	}
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

	var res contracts.Result
	var err error
	for i := 0; i < infraReadyHeartbeatThreshold; i++ {
		res, err = handler.HandleNode(context.Background(), inventory)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
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
	components := inventory.Status.Bootstrap.Components
	if len(components) != 4 {
		t.Fatalf("expected GPU workloads enabled, got %+v", components)
	}
	for _, component := range []meta.Component{
		meta.ComponentValidator,
		meta.ComponentGPUFeatureDiscovery,
		meta.ComponentDCGM,
		meta.ComponentDCGMExporter,
	} {
		if !components[string(component)] {
			t.Fatalf("component %s not enabled in status: %+v", component, components)
		}
	}
	if inventory.Status.Bootstrap.PendingDevices != nil {
		t.Fatalf("expected no pending devices, got %+v", inventory.Status.Bootstrap.PendingDevices)
	}
	if inventory.Status.Bootstrap.ValidatorRequired {
		t.Fatalf("validator should not be required when node ready")
	}
}

func TestWorkloadStatusHandlerHandlesNodeWithoutHardware(t *testing.T) {
	scheme := newScheme(t)
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(fake.NewClientBuilder().WithScheme(scheme).Build())
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-b"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: false},
			Conditions: []metav1.Condition{{
				Type:   conditionInventoryComplete,
				Status: metav1.ConditionTrue,
			}},
		},
	}

	res, err := handler.HandleNode(context.Background(), inventory)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if res.RequeueAfter != defaultNotReadyRequeueDelay {
		t.Fatalf("expected fast requeue for nodes without hardware, got %s", res.RequeueAfter)
	}
	cond := findCondition(inventory.Status.Conditions, conditionReadyForPooling)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != reasonNoDevices {
		t.Fatalf("expected ReadyForPooling=false with reason NoDevices, got %+v", cond)
	}
}

func TestWorkloadStatusHandlerKeepsComponentsUntilInventoryComplete(t *testing.T) {
	scheme := newScheme(t)
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(fake.NewClientBuilder().WithScheme(scheme).Build())
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-preinv"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: false},
			// inventoryComplete is not set yet.
		},
	}

	if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if len(inventory.Status.Bootstrap.Components) == 0 {
		t.Fatalf("expected components to stay enabled before inventory completion")
	}
	if _, ok := inventory.Status.Bootstrap.Components[string(meta.ComponentValidator)]; !ok {
		t.Fatalf("expected validator to stay enabled prior to inventory completion")
	}
}

func TestWorkloadStatusHandlerMarksDegradedWorkloads(t *testing.T) {
	scheme := newScheme(t)
	node := "worker-degraded"
	gfd := readyPod("gfd", appGPUFeatureDiscovery, node)
	dcgm := readyPod("dcgm", appDCGM, node)
	exporter := readyPod("exporter", appDCGMExporter, node)
	exporter.Status.PodIP = "127.0.0.2"

	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(gfd, dcgm, exporter).Build())
	handler.fetchHeartbeat = func(context.Context, *corev1.Pod) (*metav1.Time, error) {
		ts := metav1.NewTime(time.Unix(1800, 0))
		return &ts, nil
	}
	handler.clock = func() time.Time { return time.Unix(1800, 0) }

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{Present: true},
			Devices:  v1alpha1.GPUNodeDeviceSummary{InUse: 1},
			Conditions: []metav1.Condition{{
				Type:   conditionInventoryComplete,
				Status: metav1.ConditionTrue,
			}},
		},
	}

	res, err := handler.HandleNode(context.Background(), inventory)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if res.RequeueAfter != defaultNotReadyRequeueDelay {
		t.Fatalf("expected not-ready requeue, got %s", res.RequeueAfter)
	}

	driverCond := findCondition(inventory.Status.Conditions, conditionDriverMissing)
	if driverCond == nil || driverCond.Status != metav1.ConditionTrue {
		t.Fatalf("expected DriverMissing=true, got %+v", driverCond)
	}
	infraCond := findCondition(inventory.Status.Conditions, conditionInfraDegraded)
	if infraCond == nil || infraCond.Status != metav1.ConditionTrue {
		t.Fatalf("expected InfraDegraded=true, got %+v", infraCond)
	}
	degradedCond := findCondition(inventory.Status.Conditions, conditionDegradedWorkloads)
	if degradedCond == nil || degradedCond.Status != metav1.ConditionTrue {
		t.Fatalf("expected DegradedWorkloads=true, got %+v", degradedCond)
	}
	readyCond := findCondition(inventory.Status.Conditions, conditionReadyForPooling)
	if readyCond == nil || readyCond.Reason != reasonDriverNotDetected {
		t.Fatalf("expected ReadyForPooling blocked by driver, got %+v", readyCond)
	}
}

func TestWorkloadStatusHandlerDetectsStaleHeartbeat(t *testing.T) {
	scheme := newScheme(t)
	node := "worker-heartbeat"

	dcgm := readyPod("dcgm", appDCGM, node)
	exporter := readyPod("exporter", appDCGMExporter, node)
	exporter.Status.PodIP = "127.0.0.1"

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(dcgm, exporter).Build()

	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(client)
	handler.clock = func() time.Time { return time.Unix(2000, 0) }
	handler.fetchHeartbeat = func(context.Context, *corev1.Pod) (*metav1.Time, error) {
		stale := metav1.NewTime(time.Unix(0, 0))
		return &stale, nil
	}

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

	cond := findCondition(inventory.Status.Conditions, conditionMonitoringMissing)
	if cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("expected monitoring ready condition, got %+v", cond)
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

	if inventory.Status.Bootstrap.Phase != v1alpha1.GPUNodeBootstrapPhaseMonitoring {
		t.Fatalf("expected phase Monitoring, got %s", inventory.Status.Bootstrap.Phase)
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

func TestWorkloadStatusHandlerKeepsWorkloadsDuringRevalidation(t *testing.T) {
	scheme := newScheme(t)
	node := "worker-revalidate"

	objs := []runtime.Object{
		readyPod("gfd", appGPUFeatureDiscovery, node),
		readyPod("validator", appValidator, node),
		readyPod("dcgm", appDCGM, node),
		readyPod("exporter", appDCGMExporter, node),
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(client)

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{
				Present: true,
				Devices: []v1alpha1.GPUNodeDevice{
					{InventoryID: "gpu-ready", State: v1alpha1.GPUDeviceStateReady},
					{InventoryID: "gpu-new", State: v1alpha1.GPUDeviceStateDiscovered},
				},
			},
			Bootstrap: v1alpha1.GPUNodeBootstrapStatus{
				Phase: v1alpha1.GPUNodeBootstrapPhaseMonitoring,
				Components: map[string]bool{
					string(meta.ComponentGPUFeatureDiscovery): true,
					string(meta.ComponentDCGM):                true,
					string(meta.ComponentDCGMExporter):        true,
				},
			},
		},
	}
	inventory.Status.Conditions = []metav1.Condition{{
		Type:   conditionInventoryComplete,
		Status: metav1.ConditionTrue,
	}}

	if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if inventory.Status.Bootstrap.Phase != v1alpha1.GPUNodeBootstrapPhaseValidating {
		t.Fatalf("expected phase to revert to Validating, got %s", inventory.Status.Bootstrap.Phase)
	}
	if comps := inventory.Status.Bootstrap.Components; len(comps) != 1 || !comps[string(meta.ComponentValidator)] {
		t.Fatalf("expected only validator enabled during revalidation, got %+v", comps)
	}
	if inventory.Status.Bootstrap.PendingDevices == nil || len(inventory.Status.Bootstrap.PendingDevices) != 1 || inventory.Status.Bootstrap.PendingDevices[0] != "gpu-new" {
		t.Fatalf("unexpected pending devices: %+v", inventory.Status.Bootstrap.PendingDevices)
	}
	if !inventory.Status.Bootstrap.ValidatorRequired {
		t.Fatalf("validator must be required when pending devices present")
	}
}

func TestWorkloadStatusHandlerThrottlesValidatorOnRepeatedFailures(t *testing.T) {
	scheme := newScheme(t)
	node := "worker-throttle"

	objs := []runtime.Object{
		readyPod("validator", appValidator, node),
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(client)

	now := time.Unix(1700001000, 0)
	handler.clock = func() time.Time { return now }

	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: node},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{
				Present: true,
				Devices: []v1alpha1.GPUNodeDevice{
					{InventoryID: "gpu-throttle", State: v1alpha1.GPUDeviceStateDiscovered},
				},
			},
		},
	}
	inventory.Status.Conditions = []metav1.Condition{{
		Type:   conditionInventoryComplete,
		Status: metav1.ConditionTrue,
	}}

	for i := 0; i < maxValidatorAttempts; i++ {
		if _, err := handler.HandleNode(context.Background(), inventory); err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		now = now.Add(validatorRetryInterval)
	}

	if len(inventory.Status.Bootstrap.Validations) != 1 {
		t.Fatalf("expected single validation entry, got %+v", inventory.Status.Bootstrap.Validations)
	}
	state := inventory.Status.Bootstrap.Validations[0]
	if state.Attempts != maxValidatorAttempts {
		t.Fatalf("expected %d attempts, got %d", maxValidatorAttempts, state.Attempts)
	}
	if inventory.Status.Bootstrap.ValidatorRequired {
		t.Fatalf("validator should not be required after throttling")
	}
	cond := findCondition(inventory.Status.Conditions, conditionReadyForPooling)
	if cond == nil || !strings.Contains(cond.Message, "manual intervention") {
		t.Fatalf("ready condition should mention manual intervention, got %+v", cond)
	}
	if len(inventory.Status.Bootstrap.PendingDevices) != 1 || inventory.Status.Bootstrap.PendingDevices[0] != "gpu-throttle" {
		t.Fatalf("pending devices should remain listed, got %+v", inventory.Status.Bootstrap.PendingDevices)
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
	if msg := monitoringMessage(true, false, componentStatus{}, componentStatus{}); !strings.Contains(msg, "ready") {
		t.Fatalf("unexpected ready message: %s", msg)
	}
	if msg := monitoringMessage(false, false, componentStatus{Ready: false, Message: "hostengine pending"}, componentStatus{}); !strings.Contains(msg, "hostengine pending") {
		t.Fatalf("unexpected dcgm message: %s", msg)
	}
	if msg := monitoringMessage(false, false, componentStatus{Ready: true}, componentStatus{Ready: false, Message: "exporter pending"}); !strings.Contains(msg, "exporter pending") {
		t.Fatalf("unexpected exporter message: %s", msg)
	}
	if msg := monitoringMessage(false, false, componentStatus{Ready: true}, componentStatus{Ready: true}); !strings.Contains(msg, "heartbeat") {
		t.Fatalf("unexpected fallback message: %s", msg)
	}
	if msg := monitoringMessage(true, true, componentStatus{}, componentStatus{}); !strings.Contains(msg, "waiting for stable heartbeat") {
		t.Fatalf("unexpected stabilising message: %s", msg)
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
	if ready, reason, _ := handler.evaluateReadyForPooling(inventory, true, true, true, true, true, 0, nil); ready || reason != reasonNoDevices {
		t.Fatalf("expected reason %s, got ready=%t reason=%s", reasonNoDevices, ready, reason)
	}

	// Node disabled by label.
	inventory = makeInventory()
	inventory.Status.Conditions = []metav1.Condition{{Type: conditionManagedDisabled, Status: metav1.ConditionTrue}}
	if ready, reason, _ := handler.evaluateReadyForPooling(inventory, true, true, true, true, true, 0, nil); ready || reason != reasonNodeDisabled {
		t.Fatalf("expected reason %s, got ready=%t reason=%s", reasonNodeDisabled, ready, reason)
	}

	// Inventory incomplete blocks readiness.
	inventory = makeInventory()
	if ready, reason, _ := handler.evaluateReadyForPooling(inventory, false, true, true, true, true, 0, nil); ready || reason != reasonInventoryPending {
		t.Fatalf("expected reason %s for incomplete inventory, got ready=%t reason=%s", reasonInventoryPending, ready, reason)
	}

	// Devices pending validation block readiness.
	pendingInventory := makeInventory()
	pendingInventory.Status.Hardware.Devices = []v1alpha1.GPUNodeDevice{{State: v1alpha1.GPUDeviceStateDiscovered}}
	if ready, reason, _ := handler.evaluateReadyForPooling(pendingInventory, true, true, true, true, true, 1, nil); ready || reason != reasonDevicesPending {
		t.Fatalf("expected reason %s when GPUs need validation, got ready=%t reason=%s", reasonDevicesPending, ready, reason)
	}

	// Driver/toolkit/component/monitoring issues.
	check := func(driver, toolkit, component, monitoring bool, expected string) {
		if ready, reason, _ := handler.evaluateReadyForPooling(makeInventory(), true, driver, toolkit, component, monitoring, 0, nil); ready || reason != expected {
			t.Fatalf("expected %s, got ready=%t reason=%s", expected, ready, reason)
		}
	}
	check(false, true, true, true, reasonDriverNotDetected)
	check(true, false, true, true, reasonToolkitNotReady)
	check(true, true, false, true, reasonComponentPending)
	check(true, true, true, false, reasonMonitoringUnhealthy)

	// Faulted devices block readiness.
	withFaulted := makeInventory()
	withFaulted.Status.Devices.Faulted = 1
	if ready, reason, _ := handler.evaluateReadyForPooling(withFaulted, true, true, true, true, true, 0, nil); ready || reason != reasonDevicesFaulted {
		t.Fatalf("expected %s when devices faulted, got ready=%t reason=%s", reasonDevicesFaulted, ready, reason)
	}

	// Happy path.
	if ready, reason, _ := handler.evaluateReadyForPooling(makeInventory(), true, true, true, true, true, 0, nil); !ready || reason != reasonAllChecksPassed {
		t.Fatalf("expected ReadyForPooling, got ready=%t reason=%s", ready, reason)
	}
}

func TestUpdateBootstrapStatusCopiesHeartbeat(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	inventory := &v1alpha1.GPUNodeInventory{}
	ts := metav1.NewTime(time.Unix(4242, 0))
	handler.updateBootstrapStatus(inventory, true, true, true, true, &ts, []v1alpha1.GPUNodeBootstrapWorkloadStatus{{Name: "validator"}})

	if inventory.Status.Monitoring.LastHeartbeat == nil {
		t.Fatalf("expected heartbeat to be recorded")
	}
	if inventory.Status.Monitoring.LastHeartbeat == &ts {
		t.Fatalf("heartbeat pointer must be copied to avoid aliasing")
	}
	if len(inventory.Status.Bootstrap.Workloads) != 1 {
		t.Fatalf("workloads not stored")
	}
}

func TestUpdateComponentEnablementDisablesGpuWorkloadsWithoutDevices(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
		inventory := &v1alpha1.GPUNodeInventory{
			Status: v1alpha1.GPUNodeInventoryStatus{
				Hardware: v1alpha1.GPUNodeHardware{Present: false},
				Conditions: []metav1.Condition{{
					Type:   conditionInventoryComplete,
					Status: metav1.ConditionTrue,
				}},
				Bootstrap: v1alpha1.GPUNodeBootstrapStatus{
					Components: map[string]bool{
						string(meta.ComponentGPUFeatureDiscovery): true,
						string(meta.ComponentDCGM):                true,
					},
			},
		},
	}

	handler.updateComponentEnablement(inventory, v1alpha1.GPUNodeBootstrapPhaseReady, false)
	if inventory.Status.Bootstrap.Components != nil {
		t.Fatalf("expected GPU workloads removed when no hardware present: %+v", inventory.Status.Bootstrap.Components)
	}
}

func TestReconcileValidationAttemptsNoPending(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	inventory := &v1alpha1.GPUNodeInventory{
		Status: v1alpha1.GPUNodeInventoryStatus{
			Bootstrap: v1alpha1.GPUNodeBootstrapStatus{
				Validations: []v1alpha1.GPUNodeValidationState{{InventoryID: "gpu-a", Attempts: 3}},
			},
		},
	}

	active, throttled := handler.reconcileValidationAttempts(inventory, nil, true)
	if len(active) != 0 || len(throttled) != 0 {
		t.Fatalf("expected no active or throttled devices when pending list empty")
	}
	if inventory.Status.Bootstrap.Validations != nil {
		t.Fatalf("expected validations cleared when nothing pending, got %+v", inventory.Status.Bootstrap.Validations)
	}
}

func TestReconcileValidationAttemptsThrottlesAfterLimit(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	now := time.Unix(10_000, 0)
	handler.clock = func() time.Time { return now }

	inventory := &v1alpha1.GPUNodeInventory{
		Status: v1alpha1.GPUNodeInventoryStatus{
			Bootstrap: v1alpha1.GPUNodeBootstrapStatus{
				Validations: []v1alpha1.GPUNodeValidationState{{InventoryID: "gpu-a", Attempts: maxValidatorAttempts}},
			},
		},
	}

	active, throttled := handler.reconcileValidationAttempts(inventory, []string{"gpu-a"}, true)
	if len(active) != 0 {
		t.Fatalf("expected throttled device to be excluded from active: %v", active)
	}
	if len(throttled) != 1 || throttled[0] != "gpu-a" {
		t.Fatalf("expected gpu-a throttled, got %v", throttled)
	}
}

func TestReconcileValidationAttemptsCreatesTrackingState(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	now := time.Unix(200, 0)
	handler.clock = func() time.Time { return now }
	inventory := &v1alpha1.GPUNodeInventory{}

	active, throttled := handler.reconcileValidationAttempts(inventory, []string{"gpu-c"}, true)
	if len(active) != 1 || active[0] != "gpu-c" {
		t.Fatalf("expected gpu-c active, got %v", active)
	}
	if len(throttled) != 0 {
		t.Fatalf("expected no throttled devices, got %v", throttled)
	}
	states := inventory.Status.Bootstrap.Validations
	if len(states) != 1 {
		t.Fatalf("expected single validation state, got %v", states)
	}
	if states[0].Attempts != 1 || states[0].InventoryID != "gpu-c" {
		t.Fatalf("unexpected validation state: %+v", states[0])
	}
	if states[0].LastFailure == nil || states[0].LastFailure.Unix() != now.Unix() {
		t.Fatalf("expected last failure timestamp recorded, got %+v", states[0].LastFailure)
	}
}

func TestReconcileValidationAttemptsCleansStaleEntries(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	inventory := &v1alpha1.GPUNodeInventory{
		Status: v1alpha1.GPUNodeInventoryStatus{
			Bootstrap: v1alpha1.GPUNodeBootstrapStatus{
				Validations: []v1alpha1.GPUNodeValidationState{{InventoryID: "gpu-stale", Attempts: 2}},
			},
		},
	}

	active, throttled := handler.reconcileValidationAttempts(inventory, []string{"gpu-fresh"}, true)
	if len(active) != 1 || active[0] != "gpu-fresh" {
		t.Fatalf("expected gpu-fresh to remain active, got %v", active)
	}
	if len(throttled) != 0 {
		t.Fatalf("did not expect throttled devices: %v", throttled)
	}
	for _, state := range inventory.Status.Bootstrap.Validations {
		if state.InventoryID == "gpu-stale" {
			t.Fatalf("stale validation state was not removed: %+v", inventory.Status.Bootstrap.Validations)
		}
	}
}

func TestReconcileValidationAttemptsTracksWhenValidatorNotReady(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	inventory := &v1alpha1.GPUNodeInventory{}
	active, throttled := handler.reconcileValidationAttempts(inventory, []string{"gpu-b"}, false)
	if len(active) != 1 || active[0] != "gpu-b" {
		t.Fatalf("expected gpu-b active even when validator not ready, got %v", active)
	}
	if len(throttled) != 0 {
		t.Fatalf("did not expect throttled devices: %v", throttled)
	}
	if inventory.Status.Bootstrap.Validations == nil || len(inventory.Status.Bootstrap.Validations) != 1 {
		t.Fatalf("expected validation entry created")
	}
	state := inventory.Status.Bootstrap.Validations[0]
	if state.InventoryID != "gpu-b" || state.Attempts != 0 || state.LastFailure != nil {
		t.Fatalf("unexpected validation state: %+v", state)
	}
}

func TestReconcileValidationAttemptsSkipsRetryUntilInterval(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	now := time.Unix(1_000_000, 0)
	handler.clock = func() time.Time { return now }
	lastFailure := metav1.NewTime(now.Add(-validatorRetryInterval / 2))
	inventory := &v1alpha1.GPUNodeInventory{
		Status: v1alpha1.GPUNodeInventoryStatus{
			Bootstrap: v1alpha1.GPUNodeBootstrapStatus{
				Validations: []v1alpha1.GPUNodeValidationState{{
					InventoryID: "gpu-r",
					Attempts:    1,
					LastFailure: &lastFailure,
				}},
			},
		},
	}

	active, throttled := handler.reconcileValidationAttempts(inventory, []string{"gpu-r"}, true)
	if len(throttled) != 0 || len(active) != 1 || active[0] != "gpu-r" {
		t.Fatalf("expected gpu-r active with no throttling, got active=%v throttled=%v", active, throttled)
	}
	state := inventory.Status.Bootstrap.Validations[0]
	if state.Attempts != 1 {
		t.Fatalf("expected attempts unchanged until retry interval, got %d", state.Attempts)
	}
}

func TestFlattenValidationStatesEmpty(t *testing.T) {
	if out := flattenValidationStates(nil); out != nil {
		t.Fatalf("expected nil for empty map, got %v", out)
	}
}

func TestScrapeExporterHeartbeatSuccess(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "dcgm_exporter_last_update_time_seconds 123.0")
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	host, portStr, _ := strings.Cut(u.Host, ":")
	port, _ := strconv.Atoi(portStr)

	oldClient := exporterHTTPClient
	exporterHTTPClient = server.Client()
	defer func() { exporterHTTPClient = oldClient }()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "dcgm-exporter"},
		Status:     corev1.PodStatus{PodIP: host},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "dcgm-exporter",
				Ports: []corev1.ContainerPort{{ContainerPort: int32(port)}},
			}},
		},
	}

	hb, err := scrapeExporterHeartbeat(context.Background(), pod)
	if err != nil {
		t.Fatalf("scrape failed: %v", err)
	}
	if hb == nil || hb.Time.IsZero() {
		t.Fatalf("expected heartbeat timestamp")
	}
}

func TestScrapeExporterHeartbeatErrorsWithoutIP(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "dcgm-exporter"}}
	if _, err := scrapeExporterHeartbeat(context.Background(), pod); err == nil {
		t.Fatalf("expected error when pod has no IP")
	}
}

func TestParseHeartbeatMetric(t *testing.T) {
	ts, err := parseHeartbeatMetric(strings.NewReader("dcgm_exporter_last_update_time_seconds 42.5\n"))
	if err != nil {
		t.Fatalf("expected metric to parse: %v", err)
	}
	if ts.IsZero() {
		t.Fatalf("timestamp not parsed")
	}
}

func TestParseHeartbeatMetricMissing(t *testing.T) {
	if _, err := parseHeartbeatMetric(bytes.NewBufferString("other_metric 1\n")); err == nil {
		t.Fatalf("expected error when metric missing")
	}
}

func TestParseHeartbeatMetricMissingValue(t *testing.T) {
	if _, err := parseHeartbeatMetric(strings.NewReader("dcgm_exporter_last_update_time_seconds\n")); err == nil {
		t.Fatalf("expected error when value missing")
	}
}

func TestParseHeartbeatMetricMalformedNumber(t *testing.T) {
	if _, err := parseHeartbeatMetric(strings.NewReader("dcgm_exporter_last_update_time_seconds abc\n")); err == nil {
		t.Fatalf("expected parse error for malformed value")
	}
}

func TestParseHeartbeatMetricScannerError(t *testing.T) {
	if _, err := parseHeartbeatMetric(errorReader{}); err == nil {
		t.Fatalf("expected scanner error")
	}
}

func TestExporterPortDefault(t *testing.T) {
	pod := &corev1.Pod{}
	if port := exporterPort(pod); port != 9400 {
		t.Fatalf("expected default port, got %d", port)
	}
}

func TestPendingDeviceIDsFallbacks(t *testing.T) {
	inventory := &v1alpha1.GPUNodeInventory{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-f"},
		Status: v1alpha1.GPUNodeInventoryStatus{
			Hardware: v1alpha1.GPUNodeHardware{
				Devices: []v1alpha1.GPUNodeDevice{
					{InventoryID: "gpu-0", State: v1alpha1.GPUDeviceStateDiscovered},
					{UUID: "uuid-1", State: v1alpha1.GPUDeviceStateFaulted},
					{State: v1alpha1.GPUDeviceStateFaulted},
				},
			},
		},
	}

	ids := pendingDeviceIDs(inventory)
	if len(ids) != 3 {
		t.Fatalf("expected 3 pending ids, got %v", ids)
	}
	if ids[0] != "gpu-0" || ids[1] != "uuid-1" || ids[2] != "worker-f#2" {
		t.Fatalf("unexpected fallback order: %v", ids)
	}
}

func TestUpdateBootstrapStatusClearsStaleHeartbeat(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	old := metav1.NewTime(time.Unix(111, 0))
	inventory := &v1alpha1.GPUNodeInventory{
		Status: v1alpha1.GPUNodeInventoryStatus{
			Monitoring: v1alpha1.GPUNodeMonitoring{LastHeartbeat: &old},
		},
	}

	handler.updateBootstrapStatus(inventory, true, true, true, false, nil, nil)
	if inventory.Status.Monitoring.LastHeartbeat != nil {
		t.Fatalf("expected heartbeat cleared when monitoring not ready")
	}
}

func TestReconcileValidationAttemptsSkipsRecentFailures(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	now := time.Unix(100, 0)
	handler.clock = func() time.Time { return now }
	last := metav1.NewTime(now)
	inventory := &v1alpha1.GPUNodeInventory{
		Status: v1alpha1.GPUNodeInventoryStatus{
			Bootstrap: v1alpha1.GPUNodeBootstrapStatus{
				Validations: []v1alpha1.GPUNodeValidationState{{InventoryID: "gpu-r", Attempts: 1, LastFailure: &last}},
			},
		},
	}

	active, throttled := handler.reconcileValidationAttempts(inventory, []string{"gpu-r"}, true)
	if len(active) != 1 || len(throttled) != 0 {
		t.Fatalf("expected gpu-r to stay active, got active=%v throttled=%v", active, throttled)
	}
	if got := inventory.Status.Bootstrap.Validations[0].Attempts; got != 1 {
		t.Fatalf("attempts should not change due to retry interval, got %d", got)
	}
}

func TestExporterHeartbeatListError(t *testing.T) {
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(&failingListClient{err: errors.New("boom")})
	if _, err := handler.exporterHeartbeat(context.Background(), "node-x"); err == nil {
		t.Fatalf("expected list error")
	}
}

func TestExporterHeartbeatPodNotFound(t *testing.T) {
	scheme := newScheme(t)
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)
	handler.SetClient(fake.NewClientBuilder().WithScheme(scheme).Build())
	if _, err := handler.exporterHeartbeat(context.Background(), "node-x"); err == nil {
		t.Fatalf("expected not found error when pod missing")
	}
}

func TestExporterHeartbeatSuccess(t *testing.T) {
	scheme := newScheme(t)
	handler := NewWorkloadStatusHandler(testr.New(t), meta.WorkloadsNamespace)

	otherNode := readyPod("exporter-other", appDCGMExporter, "worker-b")
	otherNode.Status.PodIP = "10.0.0.10"

	noIP := readyPod("exporter-noip", appDCGMExporter, "worker-a")
	noIP.Status.PodIP = ""

	notReady := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "exporter-pending",
			Namespace: meta.WorkloadsNamespace,
			Labels:    map[string]string{"app": appDCGMExporter},
		},
		Spec:   corev1.PodSpec{NodeName: "worker-a"},
		Status: corev1.PodStatus{PodIP: "10.0.0.11"},
	}

	ready := readyPod("exporter-ready", appDCGMExporter, "worker-a")
	ready.Status.PodIP = "10.0.0.12"

	handler.SetClient(fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(otherNode, noIP, notReady, ready).
		Build())

	var called int
	handler.fetchHeartbeat = func(context.Context, *corev1.Pod) (*metav1.Time, error) {
		called++
		ts := metav1.NewTime(time.Unix(7777, 0))
		return &ts, nil
	}

	hb, err := handler.exporterHeartbeat(context.Background(), "worker-a")
	if err != nil {
		t.Fatalf("expected heartbeat success, got error: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected single heartbeat invocation, got %d", called)
	}
	if hb == nil || hb.Time.Unix() != 7777 {
		t.Fatalf("unexpected heartbeat timestamp: %+v", hb)
	}
}

func TestScrapeExporterHeartbeatHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	host, portStr, _ := strings.Cut(u.Host, ":")
	port, _ := strconv.Atoi(portStr)

	oldClient := exporterHTTPClient
	exporterHTTPClient = server.Client()
	defer func() { exporterHTTPClient = oldClient }()

	pod := &corev1.Pod{
		Status: corev1.PodStatus{PodIP: host},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "dcgm-exporter", Ports: []corev1.ContainerPort{{ContainerPort: int32(port)}}}},
		},
	}
	if _, err := scrapeExporterHeartbeat(context.Background(), pod); err == nil {
		t.Fatalf("expected error on non-200 response")
	}
}

func TestScrapeExporterHeartbeatTransportError(t *testing.T) {
	oldClient := exporterHTTPClient
	exporterHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport error")
	})}
	defer func() { exporterHTTPClient = oldClient }()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "dcgm-exporter"},
		Status:     corev1.PodStatus{PodIP: "127.0.0.1"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "dcgm-exporter"}},
		},
	}

	if _, err := scrapeExporterHeartbeat(context.Background(), pod); err == nil {
		t.Fatalf("expected transport error")
	}
}

func TestScrapeExporterHeartbeatBadURL(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "dcgm-exporter"},
		Status:     corev1.PodStatus{PodIP: "bad host name"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "dcgm-exporter"}},
		},
	}

	if _, err := scrapeExporterHeartbeat(context.Background(), pod); err == nil {
		t.Fatalf("expected error from invalid request URL")
	}
}

func TestScrapeExporterHeartbeatParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "other_metric 1")
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	host, portStr, _ := strings.Cut(u.Host, ":")
	port, _ := strconv.Atoi(portStr)

	oldClient := exporterHTTPClient
	exporterHTTPClient = server.Client()
	defer func() { exporterHTTPClient = oldClient }()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "dcgm-exporter"},
		Status:     corev1.PodStatus{PodIP: host},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "dcgm-exporter", Ports: []corev1.ContainerPort{{ContainerPort: int32(port)}}}},
		},
	}

	ts, err := scrapeExporterHeartbeat(context.Background(), pod)
	if err != nil {
		t.Fatalf("expected fallback timestamp when heartbeat metric missing, got error: %v", err)
	}
	if ts == nil || ts.Time.IsZero() {
		t.Fatalf("expected non-nil timestamp fallback")
	}
}

func TestParseHeartbeatMetricBadValue(t *testing.T) {
	if _, err := parseHeartbeatMetric(strings.NewReader("dcgm_exporter_last_update_time_seconds NaN\n")); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestParseHeartbeatMetricInfinite(t *testing.T) {
	if _, err := parseHeartbeatMetric(strings.NewReader("dcgm_exporter_last_update_time_seconds +Inf\n")); err == nil {
		t.Fatalf("expected parse error for infinite heartbeat")
	}
}

func TestExporterPortFromContainer(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "other"},
				{
					Name:  "dcgm-exporter",
					Ports: []corev1.ContainerPort{{ContainerPort: 65000}},
				},
			},
		},
	}
	if port := exporterPort(pod); port != 65000 {
		t.Fatalf("expected container port, got %d", port)
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
	handler.updateBootstrapStatus(inventory, true, true, true, true, nil, nil)
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
		needsValidation   bool
		expected          v1alpha1.GPUNodeBootstrapPhase
	}{
		{name: "disabled", inv: &v1alpha1.GPUNodeInventory{Status: v1alpha1.GPUNodeInventoryStatus{Conditions: []metav1.Condition{{Type: conditionManagedDisabled, Status: metav1.ConditionTrue}}}}, inventoryComplete: true, validator: true, gfd: true, monitor: true, needsValidation: false, expected: v1alpha1.GPUNodeBootstrapPhaseDisabled},
		{name: "inventory-incomplete", inv: inventory, inventoryComplete: false, validator: true, gfd: true, monitor: true, needsValidation: false, expected: v1alpha1.GPUNodeBootstrapPhaseValidating},
		{name: "validator-pending", inv: inventory, inventoryComplete: true, validator: false, gfd: true, monitor: true, needsValidation: false, expected: v1alpha1.GPUNodeBootstrapPhaseValidating},
		{name: "gfd-pending", inv: inventory, inventoryComplete: true, validator: true, gfd: false, monitor: true, needsValidation: false, expected: v1alpha1.GPUNodeBootstrapPhaseMonitoring},
		{name: "monitoring", inv: inventory, inventoryComplete: true, validator: true, gfd: true, monitor: false, needsValidation: false, expected: v1alpha1.GPUNodeBootstrapPhaseMonitoring},
		{name: "ready", inv: inventory, inventoryComplete: true, validator: true, gfd: true, monitor: true, needsValidation: false, expected: v1alpha1.GPUNodeBootstrapPhaseReady},
		{name: "pending-devices-force-validating", inv: &v1alpha1.GPUNodeInventory{Status: v1alpha1.GPUNodeInventoryStatus{Bootstrap: v1alpha1.GPUNodeBootstrapStatus{Phase: v1alpha1.GPUNodeBootstrapPhaseReady}}}, inventoryComplete: true, validator: true, gfd: true, monitor: true, needsValidation: true, expected: v1alpha1.GPUNodeBootstrapPhaseValidating},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phase := determineBootstrapPhase(tt.inv, tt.inventoryComplete, tt.validator, tt.gfd, tt.monitor, tt.needsValidation)
			if phase != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, phase)
			}
		})
	}
}
