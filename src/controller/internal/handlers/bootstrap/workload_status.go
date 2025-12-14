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
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/validation"
)

const (
	conditionReadyForPooling    = "ReadyForPooling"
	conditionDriverMissing      = "DriverMissing"
	conditionToolkitMissing     = "ToolkitMissing"
	conditionMonitoringMissing  = "MonitoringMissing"
	conditionGFDReady           = "GFDReady"
	conditionManagedDisabled    = "ManagedDisabled"
	conditionInventoryComplete  = "InventoryComplete"
	conditionInfraDegraded      = "InfraDegraded"
	conditionDegradedWorkloads  = "DegradedWorkloads"
	reasonAllChecksPassed       = "AllChecksPassed"
	reasonNoDevices             = "NoDevices"
	reasonNodeDisabled          = "NodeDisabled"
	reasonDriverNotDetected     = "DriverNotDetected"
	reasonDriverDetected        = "DriverDetected"
	reasonToolkitNotReady       = "ToolkitNotReady"
	reasonToolkitReady          = "ToolkitReady"
	reasonComponentPending      = "ComponentPending"
	reasonMonitoringUnhealthy   = "MonitoringUnhealthy"
	reasonMonitoringHealthy     = "MonitoringHealthy"
	reasonComponentHealthy      = "ComponentHealthy"
	reasonInventoryPending      = "InventoryPending"
	reasonDevicesPending        = "DevicesPending"
	reasonDevicesFaulted        = "DevicesFaulted"
	reasonInfraDegraded         = "InfrastructureDegraded"
	defaultNotReadyRequeueDelay = 15 * time.Second
	defaultReadyRequeueDelay    = time.Minute

	gpuDeviceNodeLabelKey = "gpu.deckhouse.io/node"
)

var (
	appGPUFeatureDiscovery = meta.AppName(meta.ComponentGPUFeatureDiscovery)
	appValidator           = meta.AppName(meta.ComponentValidator)
	appDCGM                = meta.AppName(meta.ComponentDCGM)
	appDCGMExporter        = meta.AppName(meta.ComponentDCGMExporter)
)

// WorkloadStatusHandler evaluates health of bootstrap workloads on a node and updates inventory conditions.
type WorkloadStatusHandler struct {
	log    logr.Logger
	client client.Client
	clock  func() time.Time
}

// NewWorkloadStatusHandler creates handler that checks bootstrap workloads.
func NewWorkloadStatusHandler(log logr.Logger) *WorkloadStatusHandler {
	return &WorkloadStatusHandler{
		log:   log,
		clock: time.Now,
	}
}

func (h *WorkloadStatusHandler) SetClient(cl client.Client) {
	h.client = cl
}

func (h *WorkloadStatusHandler) Name() string {
	return "workload-status"
}

func (h *WorkloadStatusHandler) HandleNode(ctx context.Context, inventory *v1alpha1.GPUNodeState) (contracts.Result, error) {
	valStatus, valPresent := validation.StatusFromContext(ctx)
	nodeName := inventory.Spec.NodeName
	if nodeName == "" {
		nodeName = inventory.Name
	}
	if nodeName == "" {
		return contracts.Result{}, nil
	}
	if h.client == nil {
		return contracts.Result{}, fmt.Errorf("workload-status handler: client is not configured")
	}

	h.log.V(2).Info("checking bootstrap workloads", "node", nodeName)

	componentStatuses := h.buildComponentStatuses(valStatus, valPresent)
	validatorStatus := componentStatuses[appValidator]
	gfdStatus := componentStatuses[appGPUFeatureDiscovery]
	dcgmStatus := componentStatuses[appDCGM]
	exporterStatus := componentStatuses[appDCGMExporter]

	validatorReady := validatorStatus.Ready
	gfdReady := gfdStatus.Ready
	dcgmReady := dcgmStatus.Ready
	exporterReady := exporterStatus.Ready

	driverReady := valStatus.DriverReady
	toolkitReady := valStatus.ToolkitReady
	monitoringReady := valStatus.MonitoringReady
	if !valPresent {
		driverReady = validatorReady
		toolkitReady = validatorReady
		monitoringReady = dcgmReady && exporterReady
	}

	deviceList := &v1alpha1.GPUDeviceList{}
	if err := h.client.List(ctx, deviceList, client.MatchingLabels{gpuDeviceNodeLabelKey: nodeName}); err != nil {
		return contracts.Result{}, fmt.Errorf("list GPUDevices for bootstrap status: %w", err)
	}
	nodeDevices := deviceList.Items
	devicesPresent := len(nodeDevices) > 0
	stateCounters := deviceCounters(nodeDevices)

	pendingIDs := pendingDeviceIDs(nodeDevices)
	pendingDevices := len(pendingIDs)
	componentHealthy := gfdReady && validatorReady

	inventoryComplete := isInventoryComplete(inventory)
	throttledPending := []string(nil)

	requeue := defaultReadyRequeueDelay
	conditionsChanged := false

	conditionsChanged = setCondition(inventory, conditionGFDReady, componentHealthy, boolReason(componentHealthy, reasonComponentHealthy, reasonComponentPending), h.componentMessage(componentHealthy, gfdStatus, validatorStatus)) || conditionsChanged
	conditionsChanged = setCondition(inventory, conditionDriverMissing, !driverReady, boolReason(driverReady, reasonDriverDetected, reasonDriverNotDetected), driverMessage(driverReady, validatorStatus)) || conditionsChanged
	conditionsChanged = setCondition(inventory, conditionToolkitMissing, !toolkitReady, boolReason(toolkitReady, reasonToolkitReady, reasonToolkitNotReady), toolkitMessage(toolkitReady, validatorStatus)) || conditionsChanged

	monitoringMissing := !monitoringReady
	monitoringHealthy := monitoringReady
	monitoringMsg := monitoringMessage(monitoringReady, dcgmStatus, exporterStatus)
	conditionsChanged = setCondition(inventory, conditionMonitoringMissing, monitoringMissing, boolReason(monitoringReady, reasonMonitoringHealthy, reasonMonitoringUnhealthy), monitoringMsg) || conditionsChanged

	infraDegraded := !driverReady || !toolkitReady || monitoringMissing
	degradedWorkloads := infraDegraded && (stateCounters[v1alpha1.GPUDeviceStateAssigned]+stateCounters[v1alpha1.GPUDeviceStateReserved]+stateCounters[v1alpha1.GPUDeviceStateInUse]) > 0
	infraMessage := "Infrastructure components are healthy"
	if infraDegraded {
		infraMessage = "Driver/toolkit or monitoring components require attention"
	}
	conditionsChanged = setCondition(inventory, conditionInfraDegraded, infraDegraded, reasonInfraDegraded, infraMessage) || conditionsChanged
	if degradedWorkloads {
		infraMessage = "Workloads are running while infrastructure is degraded"
	} else if infraDegraded {
		infraMessage = "Infrastructure degraded; workloads not running on node"
	}
	conditionsChanged = setCondition(inventory, conditionDegradedWorkloads, degradedWorkloads, reasonInfraDegraded, infraMessage) || conditionsChanged

	nodeReady, readyReason, readyMessage := h.evaluateReadyForPooling(devicesPresent, inventory, inventoryComplete, driverReady, toolkitReady, componentHealthy, monitoringHealthy, stateCounters, pendingDevices, throttledPending)
	conditionsChanged = setCondition(inventory, conditionReadyForPooling, nodeReady, readyReason, readyMessage) || conditionsChanged

	if !nodeReady {
		requeue = defaultNotReadyRequeueDelay
	}

	if conditionsChanged {
		h.log.Info("updated bootstrap conditions", "node", nodeName, "ready", nodeReady)
	}

	return contracts.Result{RequeueAfter: requeue}, nil
}

type componentStatus struct {
	Ready   bool
	Message string
}

func (h *WorkloadStatusHandler) buildComponentStatuses(status validation.Result, present bool) map[string]componentStatus {
	componentStatuses := map[string]componentStatus{
		appGPUFeatureDiscovery: {Ready: status.GFDReady},
		appValidator:           {Ready: status.DriverReady && status.ToolkitReady},
		appDCGM:                {Ready: status.DCGMReady},
		appDCGMExporter:        {Ready: status.DCGMExporterReady},
	}

	message := status.Message
	if message == "" && !status.Ready {
		if present {
			message = "validation workloads not ready"
		} else {
			message = "validator status unavailable"
		}
	}
	for key, st := range componentStatuses {
		if !st.Ready {
			st.Message = message
		}
		componentStatuses[key] = st
	}

	return componentStatuses
}

func (h *WorkloadStatusHandler) evaluateReadyForPooling(devicesPresent bool, inventory *v1alpha1.GPUNodeState, inventoryComplete, driverReady, toolkitReady, componentReady, monitoringHealthy bool, stateCounters map[v1alpha1.GPUDeviceState]int32, pendingDevices int, throttled []string) (bool, string, string) {
	if !devicesPresent {
		return false, reasonNoDevices, "GPU devices are not detected on the node"
	}

	if cond := apimeta.FindStatusCondition(inventory.Status.Conditions, conditionManagedDisabled); cond != nil && cond.Status == metav1.ConditionTrue {
		return false, reasonNodeDisabled, "Node is marked as disabled for GPU management"
	}

	switch {
	case !inventoryComplete:
		return false, reasonInventoryPending, "Inventory data is incomplete, waiting for inventory controller"
	case pendingDevices > 0:
		return false, reasonDevicesPending, pendingDevicesMessage(pendingDevices, throttled)
	case !driverReady:
		return false, reasonDriverNotDetected, "NVIDIA driver version has not been reported yet"
	case !toolkitReady:
		return false, reasonToolkitNotReady, "CUDA toolkit installation is not finished"
	case !componentReady:
		return false, reasonComponentPending, "Bootstrap workloads are still initialising"
	case !monitoringHealthy:
		return false, reasonMonitoringUnhealthy, "DCGM exporter is not ready"
	case stateCounters[v1alpha1.GPUDeviceStateFaulted] > 0:
		return false, reasonDevicesFaulted, fmt.Sprintf("%d device(s) are faulted", stateCounters[v1alpha1.GPUDeviceStateFaulted])
	default:
		return true, reasonAllChecksPassed, "All bootstrap checks passed"
	}
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

func driverMessage(ok bool, validator componentStatus) string {
	if ok {
		return "Driver validation succeeded"
	}
	if validator.Message != "" {
		return fmt.Sprintf("Validator pending: %s", validator.Message)
	}
	return "Validator pod has not completed yet"
}

func toolkitMessage(ok bool, validator componentStatus) string {
	if ok {
		return "CUDA toolkit validation completed"
	}
	if validator.Message != "" {
		return fmt.Sprintf("Toolkit validation pending: %s", validator.Message)
	}
	return "Toolkit validation is still running"
}

func monitoringMessage(ok bool, dcgm, exporter componentStatus) string {
	if ok {
		return "DCGM exporter is ready"
	}
	if !dcgm.Ready {
		return fmt.Sprintf("DCGM hostengine pending: %s", dcgm.Message)
	}
	if !exporter.Ready {
		return fmt.Sprintf("DCGM exporter pending: %s", exporter.Message)
	}
	return "DCGM exporter is not ready"
}

func (h *WorkloadStatusHandler) componentMessage(ok bool, gfd, validator componentStatus) string {
	if ok {
		return "Bootstrap workloads are ready"
	}
	if !gfd.Ready {
		return fmt.Sprintf("GPU Feature Discovery pending: %s", gfd.Message)
	}
	if !validator.Ready {
		return fmt.Sprintf("Validator pending: %s", validator.Message)
	}
	return "Bootstrap workloads are still running"
}

func pendingDevicesMessage(total int, throttled []string) string {
	base := "GPU devices require validation"
	if total == 1 {
		base = "GPU device requires validation"
	}
	message := fmt.Sprintf("%d %s", total, base)
	if len(throttled) == 0 {
		return message
	}
	return fmt.Sprintf("%s; manual intervention required for %s", message, strings.Join(throttled, ", "))
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
