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

package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/validation"
)

const (
	conditionInventoryComplete = "InventoryComplete"
	conditionDriverReady       = "DriverReady"
	conditionToolkitReady      = "ToolkitReady"
	conditionMonitoringReady   = "MonitoringReady"
	conditionReadyForPooling   = "ReadyForPooling"
	conditionWorkloadsDegraded = "WorkloadsDegraded"

	reasonReady               = "Ready"
	reasonNoDevices           = "NoDevices"
	reasonInventoryIncomplete = "InventoryIncomplete"
	reasonDriverNotReady      = "DriverNotReady"
	reasonToolkitNotReady     = "ToolkitNotReady"
	reasonMonitoringNotReady  = "MonitoringNotReady"
	reasonDevicesFaulted      = "DevicesFaulted"
	reasonPendingDevices      = "PendingDevices"
	reasonWorkloadsDegraded   = "WorkloadsDegraded"
	reasonWorkloadsHealthy    = "WorkloadsHealthy"
)

// WorkloadStatusHandler evaluates health of bootstrap workloads on a node and updates GPUNodeState conditions.
type WorkloadStatusHandler struct {
	log    logr.Logger
	client client.Client
}

// NewWorkloadStatusHandler creates handler that checks bootstrap workloads.
func NewWorkloadStatusHandler(log logr.Logger) *WorkloadStatusHandler {
	return &WorkloadStatusHandler{log: log}
}

// SetClient injects Kubernetes client after manager initialisation.
func (h *WorkloadStatusHandler) SetClient(cl client.Client) {
	h.client = cl
}

func (h *WorkloadStatusHandler) Name() string {
	return "workload-status"
}

func (h *WorkloadStatusHandler) HandleNode(ctx context.Context, inventory *v1alpha1.GPUNodeState) (contracts.Result, error) {
	if h.client == nil {
		return contracts.Result{}, fmt.Errorf("workload-status handler: client is not configured")
	}

	nodeName := strings.TrimSpace(inventory.Spec.NodeName)
	if nodeName == "" {
		nodeName = strings.TrimSpace(inventory.Name)
	}
	if nodeName == "" {
		return contracts.Result{}, nil
	}

	valStatus, valPresent := validation.StatusFromContext(ctx)
	validatorMessage := strings.TrimSpace(valStatus.Message)
	if !valPresent && validatorMessage == "" {
		validatorMessage = "validator status unavailable"
	}

	driverReady := valPresent && valStatus.DriverReady
	toolkitReady := valPresent && valStatus.ToolkitReady
	monitoringReady := valPresent && valStatus.GFDReady && valStatus.DCGMReady && valStatus.DCGMExporterReady

	deviceList := &v1alpha1.GPUDeviceList{}
	if err := h.client.List(ctx, deviceList, client.MatchingFields{indexer.GPUDeviceNodeField: nodeName}); err != nil {
		return contracts.Result{}, fmt.Errorf("list GPUDevices for bootstrap status: %w", err)
	}

	devicesPresent := len(deviceList.Items) > 0
	stateCounters := deviceCounters(deviceList.Items)

	pendingIDs := pendingDeviceIDs(deviceList.Items)
	pendingDevices := len(pendingIDs)
	inventoryComplete := isInventoryComplete(inventory)

	infraReady := driverReady && toolkitReady && monitoringReady
	hasWorkloads := (stateCounters[v1alpha1.GPUDeviceStateAssigned] +
		stateCounters[v1alpha1.GPUDeviceStateReserved] +
		stateCounters[v1alpha1.GPUDeviceStateInUse]) > 0
	workloadsDegraded := !infraReady && hasWorkloads

	conditionsChanged := false
	conditionsChanged = setCondition(
		inventory,
		conditionDriverReady,
		driverReady,
		boolReason(driverReady, conditionDriverReady, reasonDriverNotReady),
		conditionMessage(driverReady, "driver is ready", validatorMessage),
	) || conditionsChanged
	conditionsChanged = setCondition(
		inventory,
		conditionToolkitReady,
		toolkitReady,
		boolReason(toolkitReady, conditionToolkitReady, reasonToolkitNotReady),
		conditionMessage(toolkitReady, "toolkit is ready", validatorMessage),
	) || conditionsChanged
	conditionsChanged = setCondition(
		inventory,
		conditionMonitoringReady,
		monitoringReady,
		boolReason(monitoringReady, conditionMonitoringReady, reasonMonitoringNotReady),
		conditionMessage(monitoringReady, "monitoring is ready", validatorMessage),
	) || conditionsChanged

	conditionsChanged = setCondition(
		inventory,
		conditionWorkloadsDegraded,
		workloadsDegraded,
		boolReason(workloadsDegraded, reasonWorkloadsDegraded, reasonWorkloadsHealthy),
		workloadsDegradedMessage(hasWorkloads, workloadsDegraded),
	) || conditionsChanged

	nodeReady, readyReason, readyMessage := evaluateReadyForPooling(
		devicesPresent,
		inventoryComplete,
		driverReady,
		toolkitReady,
		monitoringReady,
		stateCounters,
		pendingDevices,
		pendingIDs,
		validatorMessage,
	)
	conditionsChanged = setCondition(inventory, conditionReadyForPooling, nodeReady, readyReason, readyMessage) || conditionsChanged

	if conditionsChanged {
		h.log.Info("updated bootstrap conditions", "node", nodeName, "ready", nodeReady)
	}

	return contracts.Result{}, nil
}

func workloadsDegradedMessage(hasWorkloads bool, degraded bool) string {
	if !hasWorkloads {
		return "no GPU workloads detected on node"
	}
	if degraded {
		return "GPU workloads are running while infrastructure is not ready"
	}
	return "GPU workloads are running on node"
}

func evaluateReadyForPooling(devicesPresent, inventoryComplete, driverReady, toolkitReady, monitoringReady bool, stateCounters map[v1alpha1.GPUDeviceState]int32, pendingDevices int, pendingIDs []string, validatorMessage string) (bool, string, string) {
	if !devicesPresent {
		return false, reasonNoDevices, "GPU devices are not detected on the node"
	}
	if !inventoryComplete {
		return false, reasonInventoryIncomplete, "inventory data is incomplete"
	}
	if stateCounters[v1alpha1.GPUDeviceStateFaulted] > 0 {
		return false, reasonDevicesFaulted, fmt.Sprintf("%d device(s) are faulted", stateCounters[v1alpha1.GPUDeviceStateFaulted])
	}
	if pendingDevices > 0 {
		return false, reasonPendingDevices, pendingDevicesMessage(pendingDevices, pendingIDs)
	}
	if !driverReady {
		return false, reasonDriverNotReady, conditionMessage(false, "driver is ready", validatorMessage)
	}
	if !toolkitReady {
		return false, reasonToolkitNotReady, conditionMessage(false, "toolkit is ready", validatorMessage)
	}
	if !monitoringReady {
		return false, reasonMonitoringNotReady, conditionMessage(false, "monitoring is ready", validatorMessage)
	}
	return true, reasonReady, "all bootstrap checks passed"
}

func setCondition(inventory *v1alpha1.GPUNodeState, condType string, status bool, reason, message string) bool {
	desiredStatus := metav1.ConditionFalse
	if status {
		desiredStatus = metav1.ConditionTrue
	}
	condition := metav1.Condition{
		Type:               condType,
		Status:             desiredStatus,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: inventory.Generation,
	}
	previous := apimeta.FindStatusCondition(inventory.Status.Conditions, condType)
	apimeta.SetStatusCondition(&inventory.Status.Conditions, condition)
	current := apimeta.FindStatusCondition(inventory.Status.Conditions, condType)
	return previous == nil || previous.Status != current.Status || previous.Reason != current.Reason || previous.Message != current.Message
}

func isInventoryComplete(inventory *v1alpha1.GPUNodeState) bool {
	cond := apimeta.FindStatusCondition(inventory.Status.Conditions, conditionInventoryComplete)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

func boolReason(ok bool, success, failure string) string {
	if ok {
		return success
	}
	return failure
}

func conditionMessage(ok bool, okMessage, notReadyDetails string) string {
	if ok {
		return okMessage
	}
	if notReadyDetails != "" {
		return notReadyDetails
	}
	return "not ready"
}

func pendingDevicesMessage(total int, pendingIDs []string) string {
	base := "GPU devices require validation"
	if total == 1 {
		base = "GPU device requires validation"
	}
	message := fmt.Sprintf("%d %s", total, base)
	if len(pendingIDs) == 0 {
		return message
	}
	return fmt.Sprintf("%s (%s)", message, strings.Join(pendingIDs, ", "))
}

func pendingDeviceIDs(devices []v1alpha1.GPUDevice) []string {
	ids := make([]string, 0, len(devices))
	for i := range devices {
		dev := devices[i]
		if !deviceStateNeedsValidation(dev.Status.State) {
			continue
		}
		id := strings.TrimSpace(dev.Status.InventoryID)
		if id == "" {
			id = dev.Name
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func deviceStateNeedsValidation(state v1alpha1.GPUDeviceState) bool {
	switch state {
	case v1alpha1.GPUDeviceStateReady,
		v1alpha1.GPUDeviceStatePendingAssignment,
		v1alpha1.GPUDeviceStateAssigned,
		v1alpha1.GPUDeviceStateReserved,
		v1alpha1.GPUDeviceStateInUse:
		return false
	default:
		return true
	}
}

func deviceCounters(devices []v1alpha1.GPUDevice) map[v1alpha1.GPUDeviceState]int32 {
	counters := make(map[v1alpha1.GPUDeviceState]int32)
	for i := range devices {
		counters[normalizeDeviceState(devices[i].Status.State)]++
	}
	return counters
}
