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
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	bootstrapcomponents "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/components"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/bootstrap/meta"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

const (
	conditionReadyForPooling    = "ReadyForPooling"
	conditionDriverMissing      = "DriverMissing"
	conditionToolkitMissing     = "ToolkitMissing"
	conditionMonitoringMissing  = "MonitoringMissing"
	conditionGFDReady           = "GFDReady"
	conditionManagedDisabled    = "ManagedDisabled"
	conditionInventoryComplete  = "InventoryComplete"
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
	defaultNotReadyRequeueDelay = 15 * time.Second
	defaultReadyRequeueDelay    = time.Minute
)

var (
	workloadComponents = func() []workloadComponent {
		names := bootstrapcomponents.Names()
		items := make([]workloadComponent, len(names))
		for i, component := range names {
			items[i] = workloadComponent{
				component: component,
				app:       meta.AppName(component),
			}
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].app < items[j].app
		})
		return items
	}()
	componentAppNames = func() []string {
		values := make([]string, len(workloadComponents))
		for i, item := range workloadComponents {
			values[i] = item.app
		}
		return values
	}()
	appGPUFeatureDiscovery = meta.AppName(meta.ComponentGPUFeatureDiscovery)
	appValidator           = meta.AppName(meta.ComponentValidator)
	appDCGM                = meta.AppName(meta.ComponentDCGM)
	appDCGMExporter        = meta.AppName(meta.ComponentDCGMExporter)
)

type workloadComponent struct {
	component meta.Component
	app       string
}

// BootstrapComponentApps exposes managed component names for controllers.
func BootstrapComponentApps() []string {
	return append([]string(nil), componentAppNames...)
}

// WorkloadStatusHandler evaluates health of bootstrap workloads on a node and updates inventory conditions.
type WorkloadStatusHandler struct {
	log       logr.Logger
	client    client.Client
	namespace string
	clock     func() time.Time
}

// NewWorkloadStatusHandler creates handler that checks bootstrap workloads in the given namespace.
func NewWorkloadStatusHandler(log logr.Logger, namespace string) *WorkloadStatusHandler {
	if namespace == "" {
		namespace = meta.WorkloadsNamespace
	}
	return &WorkloadStatusHandler{
		log:       log,
		namespace: namespace,
		clock:     time.Now,
	}
}

// SetClient injects Kubernetes client after manager initialisation.
func (h *WorkloadStatusHandler) SetClient(cl client.Client) {
	h.client = cl
}

func (h *WorkloadStatusHandler) Name() string {
	return "workload-status"
}

func (h *WorkloadStatusHandler) HandleNode(ctx context.Context, inventory *v1alpha1.GPUNodeInventory) (contracts.Result, error) {
	if h.client == nil {
		return contracts.Result{}, fmt.Errorf("workload-status handler: client is not configured")
	}
	nodeName := inventory.Spec.NodeName
	if nodeName == "" {
		nodeName = inventory.Name
	}
	if nodeName == "" {
		return contracts.Result{}, nil
	}

	h.log.V(2).Info("checking bootstrap workloads", "node", nodeName)

	driverReady := stringsTrimNotEmpty(inventory.Status.Driver.Version)
	toolkitReady := inventory.Status.Driver.ToolkitReady

	componentStatuses, err := h.probeComponents(ctx, nodeName)
	if err != nil {
		return contracts.Result{}, err
	}

	workloads := h.buildWorkloadStatuses(componentStatuses)
	gfdReady := componentStatuses[appGPUFeatureDiscovery].Ready
	validatorReady := componentStatuses[appValidator].Ready
	dcgmReady := componentStatuses[appDCGM].Ready
	exporterReady := componentStatuses[appDCGMExporter].Ready

	monitoringReady := dcgmReady && exporterReady
	componentHealthy := gfdReady && validatorReady

	h.updateBootstrapStatus(inventory, componentHealthy, toolkitReady, monitoringReady, workloads)
	inventoryComplete := isInventoryComplete(inventory)
	phase := determineBootstrapPhase(inventory, inventoryComplete, driverReady, toolkitReady, componentHealthy, monitoringReady)
	h.setBootstrapPhase(inventory, phase)

	requeue := defaultReadyRequeueDelay
	conditionsChanged := false

	conditionsChanged = setCondition(inventory, conditionGFDReady, componentHealthy, boolReason(componentHealthy, reasonComponentHealthy, reasonComponentPending), h.componentMessage(componentHealthy, componentStatuses[appGPUFeatureDiscovery], componentStatuses[appValidator])) || conditionsChanged
	conditionsChanged = setCondition(inventory, conditionDriverMissing, !driverReady, boolReason(driverReady, reasonDriverDetected, reasonDriverNotDetected), driverMessage(driverReady)) || conditionsChanged
	conditionsChanged = setCondition(inventory, conditionToolkitMissing, !toolkitReady, boolReason(toolkitReady, reasonToolkitReady, reasonToolkitNotReady), toolkitMessage(toolkitReady)) || conditionsChanged
	conditionsChanged = setCondition(inventory, conditionMonitoringMissing, !monitoringReady, boolReason(monitoringReady, reasonMonitoringHealthy, reasonMonitoringUnhealthy), monitoringMessage(monitoringReady, componentStatuses[appDCGM], componentStatuses[appDCGMExporter])) || conditionsChanged

	nodeReady, readyReason, readyMessage := h.evaluateReadyForPooling(inventory, inventoryComplete, driverReady, toolkitReady, componentHealthy, monitoringReady)
	conditionsChanged = setCondition(inventory, conditionReadyForPooling, nodeReady, readyReason, readyMessage) || conditionsChanged

	if !nodeReady {
		requeue = defaultNotReadyRequeueDelay
	}

	if conditionsChanged {
		h.log.Info("updated bootstrap conditions", "node", nodeName, "ready", nodeReady)
	}

	return contracts.Result{RequeueAfter: requeue}, err
}

type componentStatus struct {
	Ready   bool
	Message string
}

func (h *WorkloadStatusHandler) buildWorkloadStatuses(componentStatuses map[string]componentStatus) []v1alpha1.GPUNodeBootstrapWorkloadStatus {
	workloads := make([]v1alpha1.GPUNodeBootstrapWorkloadStatus, 0, len(workloadComponents))
	for _, item := range workloadComponents {
		status := componentStatuses[item.app]
		workload := v1alpha1.GPUNodeBootstrapWorkloadStatus{
			Name:    string(item.component),
			Healthy: status.Ready,
		}
		if status.Message != "" {
			workload.Message = status.Message
		}
		workloads = append(workloads, workload)
	}
	return workloads
}

func (h *WorkloadStatusHandler) probeComponents(ctx context.Context, node string) (map[string]componentStatus, error) {
	results := make(map[string]componentStatus, len(componentAppNames))
	var errs []error

	for _, app := range componentAppNames {
		ready, msg, err := h.isPodReadyOnNode(ctx, app, node)
		if err != nil {
			errs = append(errs, fmt.Errorf("list pods for %s: %w", app, err))
			continue
		}
		results[app] = componentStatus{Ready: ready, Message: msg}
	}

	return results, utilerrors.NewAggregate(errs)
}

func (h *WorkloadStatusHandler) isPodReadyOnNode(ctx context.Context, app, node string) (bool, string, error) {
	podList := &corev1.PodList{}
	if err := h.client.List(ctx, podList, client.InNamespace(h.namespace), client.MatchingLabels{"app": app}); err != nil {
		return false, "", err
	}

	var pendingMsg string

	for _, pod := range podList.Items {
		if pod.Spec.NodeName != node {
			continue
		}
		if pod.DeletionTimestamp != nil {
			continue
		}
		if podReady(&pod) {
			return true, "", nil
		}
		pendingMsg = podPendingMessage(&pod)
	}

	if pendingMsg == "" {
		pendingMsg = "pod not scheduled on node"
	}
	return false, pendingMsg, nil
}

func (h *WorkloadStatusHandler) updateBootstrapStatus(inventory *v1alpha1.GPUNodeInventory, gfdReady, toolkitReady, monitoringReady bool, workloads []v1alpha1.GPUNodeBootstrapWorkloadStatus) {
	if inventory.Status.Bootstrap.GFDReady != gfdReady {
		inventory.Status.Bootstrap.GFDReady = gfdReady
	}
	if inventory.Status.Bootstrap.ToolkitReady != toolkitReady {
		inventory.Status.Bootstrap.ToolkitReady = toolkitReady
	}
	if inventory.Status.Bootstrap.LastRun == nil {
		inventory.Status.Bootstrap.LastRun = &metav1.Time{}
	}
	now := metav1.NewTime(h.clock().UTC())
	*inventory.Status.Bootstrap.LastRun = now

	if inventory.Status.Monitoring.DCGMReady != monitoringReady {
		inventory.Status.Monitoring.DCGMReady = monitoringReady
		if monitoringReady {
			inventory.Status.Monitoring.LastHeartbeat = &now
		}
	}
	if len(workloads) > 0 {
		inventory.Status.Bootstrap.Workloads = workloads
	} else {
		inventory.Status.Bootstrap.Workloads = nil
	}
}

func (h *WorkloadStatusHandler) setBootstrapPhase(inventory *v1alpha1.GPUNodeInventory, phase v1alpha1.GPUNodeBootstrapPhase) {
	if inventory.Status.Bootstrap.Phase != phase {
		inventory.Status.Bootstrap.Phase = phase
	}
}

func (h *WorkloadStatusHandler) evaluateReadyForPooling(inventory *v1alpha1.GPUNodeInventory, inventoryComplete, driverReady, toolkitReady, componentReady, monitoringReady bool) (bool, string, string) {
	hardwarePresent := inventory.Status.Hardware.Present || len(inventory.Status.Hardware.Devices) > 0
	if !hardwarePresent {
		return false, reasonNoDevices, "GPU devices are not detected on the node"
	}

	if cond := apimeta.FindStatusCondition(inventory.Status.Conditions, conditionManagedDisabled); cond != nil && cond.Status == metav1.ConditionTrue {
		return false, reasonNodeDisabled, "Node is marked as disabled for GPU management"
	}

	switch {
	case !inventoryComplete:
		return false, reasonInventoryPending, "Inventory data is incomplete, waiting for inventory controller"
	case !driverReady:
		return false, reasonDriverNotDetected, "NVIDIA driver version has not been reported yet"
	case !toolkitReady:
		return false, reasonToolkitNotReady, "CUDA toolkit installation is not finished"
	case !componentReady:
		return false, reasonComponentPending, "Bootstrap workloads are still initialising"
	case !monitoringReady:
		return false, reasonMonitoringUnhealthy, "GPU monitoring stack is not ready"
	default:
		return true, reasonAllChecksPassed, "All bootstrap checks passed"
	}
}

func setCondition(inventory *v1alpha1.GPUNodeInventory, condType string, status bool, reason, message string) bool {
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

func isInventoryComplete(inventory *v1alpha1.GPUNodeInventory) bool {
	cond := apimeta.FindStatusCondition(inventory.Status.Conditions, conditionInventoryComplete)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

func boolReason(ok bool, success, failure string) string {
	if ok {
		return success
	}
	return failure
}

func driverMessage(ok bool) string {
	if ok {
		return "Driver information reported by GPU Feature Discovery"
	}
	return "Driver version not reported by GPU Feature Discovery"
}

func toolkitMessage(ok bool) string {
	if ok {
		return "CUDA toolkit preparation completed"
	}
	return "Waiting for CUDA toolkit preparation to finish"
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
	return "GPU monitoring stack is not ready"
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

func determineBootstrapPhase(inventory *v1alpha1.GPUNodeInventory, inventoryComplete, driverReady, toolkitReady, componentReady, monitoringReady bool) v1alpha1.GPUNodeBootstrapPhase {
	if managedDisabled(inventory) {
		return v1alpha1.GPUNodeBootstrapPhaseDisabled
	}
	if !inventoryComplete {
		return v1alpha1.GPUNodeBootstrapPhaseValidating
	}
	if !driverReady || !toolkitReady {
		return v1alpha1.GPUNodeBootstrapPhaseValidatingFailed
	}
	if !componentReady {
		return v1alpha1.GPUNodeBootstrapPhaseGFD
	}
	if !monitoringReady {
		return v1alpha1.GPUNodeBootstrapPhaseMonitoring
	}
	return v1alpha1.GPUNodeBootstrapPhaseReady
}

func managedDisabled(inventory *v1alpha1.GPUNodeInventory) bool {
	cond := apimeta.FindStatusCondition(inventory.Status.Conditions, conditionManagedDisabled)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

func podReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func podPendingMessage(pod *corev1.Pod) string {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status != corev1.ConditionTrue {
			return cond.Message
		}
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			return fmt.Sprintf("container %s waiting: %s", cs.Name, cs.State.Waiting.Reason)
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return fmt.Sprintf("container %s terminated: %s", cs.Name, cs.State.Terminated.Reason)
		}
	}
	return "pod not ready"
}

func stringsTrimNotEmpty(value string) bool {
	return strings.TrimSpace(value) != ""
}
