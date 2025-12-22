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

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
)

// DeviceStateSyncHandler keeps GPUDevice.state in sync with bootstrap readiness.
type DeviceStateSyncHandler struct {
	log    logr.Logger
	client client.Client
}

// NewDeviceStateSyncHandler creates handler that updates device states on the node.
func NewDeviceStateSyncHandler(log logr.Logger) *DeviceStateSyncHandler {
	return &DeviceStateSyncHandler{log: log}
}

// SetClient injects Kubernetes client after manager initialisation.
func (h *DeviceStateSyncHandler) SetClient(cl client.Client) {
	h.client = cl
}

func (h *DeviceStateSyncHandler) Name() string {
	return "device-state-sync"
}

func (h *DeviceStateSyncHandler) HandleNode(ctx context.Context, inventory *v1alpha1.GPUNodeState) (reconcile.Result, error) {
	if h.client == nil {
		return reconcile.Result{}, fmt.Errorf("device-state-sync handler: client is not configured")
	}
	nodeName := inventory.Spec.NodeName
	if nodeName == "" {
		nodeName = inventory.Name
	}
	if nodeName == "" {
		return reconcile.Result{}, nil
	}
	driverReady := isConditionTrue(inventory, conditionDriverReady)
	toolkitReady := isConditionTrue(inventory, conditionToolkitReady)
	monitoringReady := isConditionTrue(inventory, conditionMonitoringReady)

	driverAndToolkitReady := driverReady && toolkitReady
	infraReady := driverReady && toolkitReady && monitoringReady

	deviceList := &v1alpha1.GPUDeviceList{}
	if err := h.client.List(ctx, deviceList, client.MatchingFields{indexer.GPUDeviceNodeField: nodeName}); err != nil {
		return reconcile.Result{}, fmt.Errorf("list devices on node %s: %w", nodeName, err)
	}

	if len(deviceList.Items) == 0 {
		return reconcile.Result{}, nil
	}

	var errs []error
	for i := range deviceList.Items {
		device := &deviceList.Items[i]
		target, mutate := desiredDeviceState(device, driverAndToolkitReady, infraReady)
		if !mutate || device.Status.State == target {
			continue
		}

		original := device.DeepCopy()
		device.Status.State = target
		patch := client.MergeFrom(original)
		if original.GetResourceVersion() != "" {
			patch = client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{})
		}
		if err := h.client.Status().Patch(ctx, device, patch); err != nil {
			errs = append(errs, fmt.Errorf("patch device %s: %w", device.Name, err))
			continue
		}
		h.log.V(1).Info("updated device state", "device", device.Name, "node", nodeName, "state", target)
	}

	return reconcile.Result{}, utilerrors.NewAggregate(errs)
}

func isConditionTrue(inventory *v1alpha1.GPUNodeState, condType string) bool {
	for _, cond := range inventory.Status.Conditions {
		if cond.Type == condType {
			return cond.Status == metav1.ConditionTrue
		}
	}
	return false
}

func desiredDeviceState(device *v1alpha1.GPUDevice, driverAndToolkitReady, infraReady bool) (v1alpha1.GPUDeviceState, bool) {
	state := normalizeDeviceState(device.Status.State)
	current := device.Status.State

	switch state {
	case v1alpha1.GPUDeviceStateAssigned,
		v1alpha1.GPUDeviceStateReserved,
		v1alpha1.GPUDeviceStateInUse:
		// Pool controllers own these transitions.
		return state, state != current
	case v1alpha1.GPUDeviceStatePendingAssignment:
		return state, state != current
	case v1alpha1.GPUDeviceStateReady:
		return state, state != current
	case v1alpha1.GPUDeviceStateFaulted:
		if driverAndToolkitReady {
			return v1alpha1.GPUDeviceStateValidating, true
		}
		return state, state != current
	case v1alpha1.GPUDeviceStateValidating:
		if infraReady {
			return v1alpha1.GPUDeviceStateReady, true
		}
		return state, state != current
	default:
		if infraReady {
			return v1alpha1.GPUDeviceStateReady, true
		}
		if driverAndToolkitReady {
			return v1alpha1.GPUDeviceStateValidating, true
		}
		return state, state != current
	}
}

func normalizeDeviceState(state v1alpha1.GPUDeviceState) v1alpha1.GPUDeviceState {
	if state == "" {
		return v1alpha1.GPUDeviceStateDiscovered
	}
	return state
}
