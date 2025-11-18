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
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
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
	reasonDevicesPending        = "DevicesPending"
	defaultNotReadyRequeueDelay = 15 * time.Second
	defaultReadyRequeueDelay    = time.Minute
	maxValidatorAttempts        = 5
	validatorRetryInterval      = time.Minute
	heartbeatStaleAfter         = 2 * time.Minute
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
	exporterHTTPClient     = &http.Client{Timeout: 3 * time.Second}
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
	log            logr.Logger
	client         client.Client
	namespace      string
	clock          func() time.Time
	fetchHeartbeat func(context.Context, *corev1.Pod) (*metav1.Time, error)
}

// NewWorkloadStatusHandler creates handler that checks bootstrap workloads in the given namespace.
func NewWorkloadStatusHandler(log logr.Logger, namespace string) *WorkloadStatusHandler {
	if namespace == "" {
		namespace = meta.WorkloadsNamespace
	}
	return &WorkloadStatusHandler{
		log:            log,
		namespace:      namespace,
		clock:          time.Now,
		fetchHeartbeat: scrapeExporterHeartbeat,
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

	componentStatuses, err := h.probeComponents(ctx, nodeName)
	if err != nil {
		return contracts.Result{}, err
	}

	workloads := h.buildWorkloadStatuses(componentStatuses)
	validatorStatus := componentStatuses[appValidator]
	gfdStatus := componentStatuses[appGPUFeatureDiscovery]
	dcgmStatus := componentStatuses[appDCGM]
	exporterStatus := componentStatuses[appDCGMExporter]

	validatorReady := validatorStatus.Ready
	gfdReady := gfdStatus.Ready
	dcgmReady := dcgmStatus.Ready
	exporterReady := exporterStatus.Ready

	pendingIDs := pendingDeviceIDs(inventory)
	activePending, throttledPending := h.reconcileValidationAttempts(inventory, pendingIDs, validatorReady)
	pendingDevices := len(pendingIDs)
	needsValidation := len(activePending) > 0

	driverReady := validatorReady
	toolkitReady := validatorReady
	componentHealthy := gfdReady && validatorReady
	var heartbeat *metav1.Time
	monitoringReady := dcgmReady && exporterReady
	if monitoringReady {
		if hb, err := h.exporterHeartbeat(ctx, nodeName); err == nil {
			heartbeat = hb
		} else {
			h.log.V(2).Info("dcgm exporter heartbeat unavailable", "node", nodeName, "error", err)
		}
	}

	h.updateBootstrapStatus(inventory, gfdReady, toolkitReady, monitoringReady, heartbeat, workloads)
	inventoryComplete := isInventoryComplete(inventory)
	phase := determineBootstrapPhase(inventory, inventoryComplete, validatorReady, gfdReady, monitoringReady, needsValidation)
	h.setBootstrapPhase(inventory, phase)
	validatorRequired := h.validatorRequired(phase, needsValidation)
	h.updateComponentEnablement(inventory, phase, validatorRequired)
	h.updatePendingDevices(inventory, pendingIDs)

	requeue := defaultReadyRequeueDelay
	conditionsChanged := false

	conditionsChanged = setCondition(inventory, conditionGFDReady, componentHealthy, boolReason(componentHealthy, reasonComponentHealthy, reasonComponentPending), h.componentMessage(componentHealthy, gfdStatus, validatorStatus)) || conditionsChanged
	conditionsChanged = setCondition(inventory, conditionDriverMissing, !driverReady, boolReason(driverReady, reasonDriverDetected, reasonDriverNotDetected), driverMessage(driverReady, validatorStatus)) || conditionsChanged
	conditionsChanged = setCondition(inventory, conditionToolkitMissing, !toolkitReady, boolReason(toolkitReady, reasonToolkitReady, reasonToolkitNotReady), toolkitMessage(toolkitReady, validatorStatus)) || conditionsChanged
	conditionsChanged = setCondition(inventory, conditionMonitoringMissing, !monitoringReady, boolReason(monitoringReady, reasonMonitoringHealthy, reasonMonitoringUnhealthy), monitoringMessage(monitoringReady, dcgmStatus, exporterStatus)) || conditionsChanged

	nodeReady, readyReason, readyMessage := h.evaluateReadyForPooling(inventory, inventoryComplete, driverReady, toolkitReady, componentHealthy, monitoringReady, pendingDevices, throttledPending)
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

func (h *WorkloadStatusHandler) updateBootstrapStatus(inventory *v1alpha1.GPUNodeInventory, gfdReady, toolkitReady, monitoringReady bool, heartbeat *metav1.Time, workloads []v1alpha1.GPUNodeBootstrapWorkloadStatus) {
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
	}
	if !monitoringReady {
		inventory.Status.Monitoring.LastHeartbeat = nil
	} else if heartbeat != nil {
		hb := *heartbeat
		inventory.Status.Monitoring.LastHeartbeat = &hb
	}
	if len(workloads) > 0 {
		inventory.Status.Bootstrap.Workloads = workloads
	} else {
		inventory.Status.Bootstrap.Workloads = nil
	}
}

func (h *WorkloadStatusHandler) updateComponentEnablement(inventory *v1alpha1.GPUNodeInventory, phase v1alpha1.GPUNodeBootstrapPhase, validatorRequired bool) {
	devicesPresent := hardwarePresent(inventory)
	enabled := bootstrapcomponents.EnabledComponents(phase, devicesPresent)
	// Keep validator DaemonSet running on GPU nodes even after initial validation
	// to match upstream behaviour and avoid tearing down follow-up workloads.
	if devicesPresent && phase != v1alpha1.GPUNodeBootstrapPhaseDisabled {
		enabled[meta.ComponentValidator] = true
	}
	if !devicesPresent {
		delete(enabled, meta.ComponentValidator)
	}

	if len(enabled) == 0 {
		inventory.Status.Bootstrap.Components = nil
	} else {
		next := make(map[string]bool, len(enabled))
		for component := range enabled {
			next[string(component)] = true
		}
		inventory.Status.Bootstrap.Components = next
	}
	inventory.Status.Bootstrap.ValidatorRequired = validatorRequired
}

func (h *WorkloadStatusHandler) updatePendingDevices(inventory *v1alpha1.GPUNodeInventory, pending []string) {
	if len(pending) == 0 {
		inventory.Status.Bootstrap.PendingDevices = nil
		return
	}
	sorted := append([]string(nil), pending...)
	sort.Strings(sorted)
	inventory.Status.Bootstrap.PendingDevices = sorted
}

func (h *WorkloadStatusHandler) reconcileValidationAttempts(inventory *v1alpha1.GPUNodeInventory, pending []string, validatorReady bool) ([]string, []string) {
	if len(pending) == 0 {
		inventory.Status.Bootstrap.Validations = nil
		return nil, nil
	}

	tracker := make(map[string]*v1alpha1.GPUNodeValidationState, len(inventory.Status.Bootstrap.Validations))
	for _, state := range inventory.Status.Bootstrap.Validations {
		copyState := v1alpha1.GPUNodeValidationState{
			InventoryID: state.InventoryID,
			Attempts:    state.Attempts,
		}
		if state.LastFailure != nil {
			ts := *state.LastFailure
			copyState.LastFailure = &ts
		}
		tracker[state.InventoryID] = &copyState
	}

	now := h.clock().UTC()
	pendingSet := make(map[string]struct{}, len(pending))
	for _, id := range pending {
		pendingSet[id] = struct{}{}
		state, ok := tracker[id]
		if !ok {
			state = &v1alpha1.GPUNodeValidationState{InventoryID: id}
			tracker[id] = state
		}
		if validatorReady && state.Attempts < maxValidatorAttempts {
			shouldRetry := state.LastFailure == nil || now.Sub(state.LastFailure.Time) >= validatorRetryInterval
			if shouldRetry {
				state.Attempts++
				ts := metav1.NewTime(now)
				state.LastFailure = &ts
			}
		}
	}

	for id := range tracker {
		if _, ok := pendingSet[id]; !ok {
			delete(tracker, id)
		}
	}

	throttled := make([]string, 0)
	active := make([]string, 0, len(pending))
	for _, id := range pending {
		state := tracker[id]
		if state == nil {
			continue
		}
		if state.Attempts >= maxValidatorAttempts {
			throttled = append(throttled, id)
		} else {
			active = append(active, id)
		}
	}
	sort.Strings(throttled)

	inventory.Status.Bootstrap.Validations = flattenValidationStates(tracker)

	return active, throttled
}

func flattenValidationStates(states map[string]*v1alpha1.GPUNodeValidationState) []v1alpha1.GPUNodeValidationState {
	if len(states) == 0 {
		return nil
	}
	keys := make([]string, 0, len(states))
	for id := range states {
		keys = append(keys, id)
	}
	sort.Strings(keys)

	result := make([]v1alpha1.GPUNodeValidationState, 0, len(keys))
	for _, id := range keys {
		state := states[id]
		copyState := v1alpha1.GPUNodeValidationState{
			InventoryID: id,
			Attempts:    state.Attempts,
		}
		if state.LastFailure != nil {
			ts := *state.LastFailure
			copyState.LastFailure = &ts
		}
		result = append(result, copyState)
	}
	return result
}

func (h *WorkloadStatusHandler) exporterHeartbeat(ctx context.Context, node string) (*metav1.Time, error) {
	podList := &corev1.PodList{}
	if err := h.client.List(ctx, podList,
		client.InNamespace(h.namespace),
		client.MatchingLabels{"app": appDCGMExporter},
	); err != nil {
		return nil, err
	}

	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Spec.NodeName != node || pod.Status.PodIP == "" {
			continue
		}
		if !podReady(pod) {
			continue
		}
		return h.fetchHeartbeat(ctx, pod)
	}

	return nil, fmt.Errorf("dcgm exporter pod for node %s not found", node)
}

func scrapeExporterHeartbeat(ctx context.Context, pod *corev1.Pod) (*metav1.Time, error) {
	if pod.Status.PodIP == "" {
		return nil, fmt.Errorf("pod %s has no IP assigned", pod.Name)
	}

	port := exporterPort(pod)
	url := fmt.Sprintf("http://%s:%d/metrics", pod.Status.PodIP, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := exporterHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	ts, err := parseHeartbeatMetric(resp.Body)
	if err != nil {
		// Some exporter builds do not expose the heartbeat metric; treat reachability as success
		// and fall back to "now" to avoid blocking bootstrap in that case.
		now := metav1.NewTime(time.Now().UTC())
		return &now, nil
	}
	heartbeat := metav1.NewTime(ts.UTC())
	return &heartbeat, nil
}

func parseHeartbeatMetric(r io.Reader) (time.Time, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "dcgm_exporter_last_update_time_seconds") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value := fields[len(fields)-1]
		seconds, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return time.Time{}, err
		}
		if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			return time.Time{}, fmt.Errorf("invalid heartbeat value %q", value)
		}
		sec := int64(seconds)
		nsec := int64((seconds - float64(sec)) * float64(time.Second))
		return time.Unix(sec, nsec), nil
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, err
	}
	return time.Time{}, fmt.Errorf("heartbeat metric not found")
}

func exporterPort(pod *corev1.Pod) int32 {
	for _, container := range pod.Spec.Containers {
		if container.Name != "dcgm-exporter" {
			continue
		}
		for _, port := range container.Ports {
			if port.ContainerPort > 0 {
				return port.ContainerPort
			}
		}
	}
	return 9400
}

func (h *WorkloadStatusHandler) setBootstrapPhase(inventory *v1alpha1.GPUNodeInventory, phase v1alpha1.GPUNodeBootstrapPhase) {
	if inventory.Status.Bootstrap.Phase != phase {
		inventory.Status.Bootstrap.Phase = phase
	}
}

func (h *WorkloadStatusHandler) evaluateReadyForPooling(inventory *v1alpha1.GPUNodeInventory, inventoryComplete, driverReady, toolkitReady, componentReady, monitoringReady bool, pendingDevices int, throttled []string) (bool, string, string) {
	if !hardwarePresent(inventory) {
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

func (h *WorkloadStatusHandler) validatorRequired(phase v1alpha1.GPUNodeBootstrapPhase, pendingDevices bool) bool {
	switch phase {
	case v1alpha1.GPUNodeBootstrapPhaseValidating,
		v1alpha1.GPUNodeBootstrapPhaseValidatingFailed:
		return true
	default:
		return pendingDevices
	}
}

func hardwarePresent(inventory *v1alpha1.GPUNodeInventory) bool {
	if inventory.Status.Hardware.Present {
		return true
	}
	return len(inventory.Status.Hardware.Devices) > 0
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
	return "DCGM exporter heartbeat has not been observed yet"
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

func determineBootstrapPhase(inventory *v1alpha1.GPUNodeInventory, inventoryComplete, validatorReady, gfdReady, monitoringReady bool, needsValidation bool) v1alpha1.GPUNodeBootstrapPhase {
	if managedDisabled(inventory) {
		return v1alpha1.GPUNodeBootstrapPhaseDisabled
	}
	if !inventoryComplete {
		return v1alpha1.GPUNodeBootstrapPhaseValidating
	}
	prev := inventory.Status.Bootstrap.Phase
	if prev == "" {
		prev = v1alpha1.GPUNodeBootstrapPhaseValidating
	}
	if needsValidation && phasePastValidation(prev) && validatorReady {
		return v1alpha1.GPUNodeBootstrapPhaseValidating
	}
	if !validatorReady {
		if phasePastValidation(prev) {
			return v1alpha1.GPUNodeBootstrapPhaseValidatingFailed
		}
		return v1alpha1.GPUNodeBootstrapPhaseValidating
	}
	if !gfdReady {
		return v1alpha1.GPUNodeBootstrapPhaseGFD
	}
	if !monitoringReady {
		return v1alpha1.GPUNodeBootstrapPhaseMonitoring
	}
	return v1alpha1.GPUNodeBootstrapPhaseReady
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

func phasePastValidation(phase v1alpha1.GPUNodeBootstrapPhase) bool {
	switch phase {
	case v1alpha1.GPUNodeBootstrapPhaseGFD,
		v1alpha1.GPUNodeBootstrapPhaseMonitoring,
		v1alpha1.GPUNodeBootstrapPhaseReady:
		return true
	default:
		return false
	}
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

func pendingDeviceIDs(inventory *v1alpha1.GPUNodeInventory) []string {
	ids := make([]string, 0, len(inventory.Status.Hardware.Devices))
	for idx, device := range inventory.Status.Hardware.Devices {
		if !deviceStateNeedsValidation(device.State) {
			continue
		}
		switch {
		case device.InventoryID != "":
			ids = append(ids, device.InventoryID)
		case device.UUID != "":
			ids = append(ids, device.UUID)
		default:
			ids = append(ids, fmt.Sprintf("%s#%d", inventory.Name, idx))
		}
	}
	return ids
}

func deviceStateNeedsValidation(state v1alpha1.GPUDeviceState) bool {
	switch state {
	case v1alpha1.GPUDeviceStateReadyForPooling,
		v1alpha1.GPUDeviceStatePendingAssignment,
		v1alpha1.GPUDeviceStateNoPoolMatched,
		v1alpha1.GPUDeviceStateAssigned,
		v1alpha1.GPUDeviceStateReserved,
		v1alpha1.GPUDeviceStateInUse:
		return false
	default:
		return true
	}
}
