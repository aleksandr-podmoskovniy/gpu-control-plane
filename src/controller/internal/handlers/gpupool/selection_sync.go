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
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

const assignmentAnnotation = "gpu.deckhouse.io/assignment"

// SelectionSyncHandler picks devices matching the pool selectors and updates pool status.
type SelectionSyncHandler struct {
	log    logr.Logger
	client client.Client
}

func NewSelectionSyncHandler(log logr.Logger, c client.Client) *SelectionSyncHandler {
	return &SelectionSyncHandler{log: log, client: c}
}

func (h *SelectionSyncHandler) Name() string {
	return "selection-sync"
}

func (h *SelectionSyncHandler) HandlePool(ctx context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	inventories := &v1alpha1.GPUNodeInventoryList{}
	if err := h.client.List(ctx, inventories); err != nil {
		return contracts.Result{}, err
	}
	devices := &v1alpha1.GPUDeviceList{}
	if err := h.client.List(ctx, devices); err != nil {
		return contracts.Result{}, err
	}

	capacityState := func(s v1alpha1.GPUDeviceState) bool {
		return s == v1alpha1.GPUDeviceStatePendingAssignment ||
			s == v1alpha1.GPUDeviceStateAssigned ||
			s == v1alpha1.GPUDeviceStateReserved ||
			s == v1alpha1.GPUDeviceStateInUse
	}

	assigned := make(map[string]v1alpha1.GPUDevice)
	for _, dev := range devices.Items {
		if dev.Annotations[assignmentAnnotation] != pool.Name {
			continue
		}
		key := dev.Status.InventoryID
		if key == "" {
			key = dev.Name
		}
		assigned[key] = dev
	}

	var selector labels.Selector
	if pool.Spec.NodeSelector != nil {
		var err error
		selector, err = metav1.LabelSelectorAsSelector(pool.Spec.NodeSelector)
		if err != nil {
			return contracts.Result{}, apierrors.NewBadRequest("invalid nodeSelector")
		}
	}

	nodeLabels := map[string]labels.Set{}
	if selector != nil {
		nodes := &corev1.NodeList{}
		if err := h.client.List(ctx, nodes); err != nil {
			return contracts.Result{}, err
		}
		for _, node := range nodes.Items {
			nodeLabels[node.Name] = labels.Set(node.Labels)
		}
	}

	var (
		totalUnits  int32
		usedUnits   int32
		baseUnits   int32
		devStatuses []v1alpha1.GPUPoolDeviceStatus
		nodeTotals  = map[string]int32{}
		nodeUsed    = map[string]int32{}
		toUpdate    []v1alpha1.GPUDevice
	)

	for _, inv := range inventories.Items {
		if selector != nil {
			lbls := labels.Set(inv.Labels)
			if nodeLbls, ok := nodeLabels[inv.Name]; ok {
				lbls = nodeLbls
			}
			if !selector.Matches(lbls) {
				continue
			}
		}
		deviceSet := inv.Status.Devices
		candidates := FilterDevices(deviceSet, pool.Spec.DeviceSelector)
		var takenOnNode int32
		for _, dev := range candidates {
			devCR, ok := assigned[dev.InventoryID]
			if !ok {
				continue
			}
			if strings.EqualFold(devCR.Labels["gpu.deckhouse.io/ignore"], "true") {
				continue
			}
			dev.State = devCR.Status.State
			autoAttach := devCR.Status.AutoAttach
			devStatuses = append(devStatuses, v1alpha1.GPUPoolDeviceStatus{
				InventoryID: dev.InventoryID,
				Node:        inv.Name,
				State:       dev.State,
				AutoAttach:  autoAttach,
			})
			if needsAssignmentUpdate(devCR, pool.Name) {
				toUpdate = append(toUpdate, devCR)
			}
			// В емкость пула учитываем только устройства, подтверждённые валидатором/DP:
			// Assigned/Reserved/InUse. PendingAssignment не добавляет слоты (по ADR).
			if capacityState(dev.State) {
				if pool.Spec.Resource.MaxDevicesPerNode != nil && takenOnNode >= *pool.Spec.Resource.MaxDevicesPerNode {
					continue
				}
				units, base := h.unitsForDevice(dev, pool)
				if units > 0 {
					totalUnits += units
					baseUnits += base
					takenOnNode++
					if dev.State == v1alpha1.GPUDeviceStateReserved || dev.State == v1alpha1.GPUDeviceStateInUse {
						usedUnits += units
						nodeUsed[inv.Name]++
					}
					nodeTotals[inv.Name]++
				}
			}
		}
	}

	// Unassign devices that still point to this pool but no longer carry the assignment annotation.
	for i := range devices.Items {
		dev := &devices.Items[i]
		if dev.Annotations[assignmentAnnotation] == pool.Name {
			continue
		}
		if dev.Status.PoolRef == nil || dev.Status.PoolRef.Name != pool.Name {
			continue
		}
		if err := h.clearDevicePool(ctx, dev.Name, pool.Name); err != nil {
			return contracts.Result{}, err
		}
	}

	pool.Status.Devices = devStatuses
	pool.Status.Capacity.Total = totalUnits
	pool.Status.Capacity.Used = usedUnits
	available := totalUnits - usedUnits
	if available < 0 {
		available = 0
	}
	pool.Status.Capacity.Available = available
	pool.Status.Capacity.Unit = pool.Spec.Resource.Unit
	pool.Status.Capacity.BaseUnits = baseUnits
	pool.Status.Capacity.SlicesPerUnit = pool.Spec.Resource.SlicesPerUnit

	pool.Status.Nodes = make([]v1alpha1.GPUPoolNodeStatus, 0, len(nodeTotals))
	for node, total := range nodeTotals {
		pool.Status.Nodes = append(pool.Status.Nodes, v1alpha1.GPUPoolNodeStatus{
			Name:            node,
			TotalDevices:    total,
			AssignedDevices: nodeUsed[node],
		})
	}

	for i := range toUpdate {
		dev := toUpdate[i]
		if err := h.assignDeviceWithRetry(ctx, dev.Name, pool.Name); err != nil {
			return contracts.Result{}, err
		}
	}

	h.log.V(2).Info("synchronised pool selection", "pool", pool.Name, "devices", len(devStatuses), "capacity", totalUnits)
	return contracts.Result{}, nil
}

func (h *SelectionSyncHandler) unitsForDevice(dev v1alpha1.GPUNodeDevice, pool *v1alpha1.GPUPool) (int32, int32) {
	if pool.Spec.Resource.Unit == "MIG" {
		if pool.Spec.Resource.MIGProfile == "" {
			return 0, 0
		}
		var profileCount int32
		for _, t := range dev.MIG.Types {
			if t.Name == pool.Spec.Resource.MIGProfile {
				profileCount += t.Count
			}
		}
		if profileCount == 0 {
			return 0, 0
		}
		if pool.Spec.Resource.SlicesPerUnit > 0 {
			return profileCount * pool.Spec.Resource.SlicesPerUnit, profileCount
		}
		return profileCount, profileCount
	}
	if pool.Spec.Resource.SlicesPerUnit > 0 {
		return pool.Spec.Resource.SlicesPerUnit, 1
	}
	return 1, 1
}

func needsAssignmentUpdate(dev v1alpha1.GPUDevice, poolName string) bool {
	if dev.Status.PoolRef == nil || dev.Status.PoolRef.Name != poolName {
		return true
	}
	if dev.Status.State == v1alpha1.GPUDeviceStateReady || dev.Status.State == v1alpha1.GPUDeviceStatePendingAssignment {
		return true
	}
	return false
}

func (h *SelectionSyncHandler) assignDeviceWithRetry(ctx context.Context, name, pool string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha1.GPUDevice{}
		if err := h.client.Get(ctx, client.ObjectKey{Name: name}, current); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		orig := current.DeepCopy()
		current.Status.PoolRef = &v1alpha1.GPUPoolReference{Name: pool}
		// Не переводим в Assigned без валидатора: Ready/Assigned переводим в PendingAssignment.
		if current.Status.State == v1alpha1.GPUDeviceStateReady || current.Status.State == v1alpha1.GPUDeviceStateAssigned {
			current.Status.State = v1alpha1.GPUDeviceStatePendingAssignment
		}
		if err := h.client.Status().Patch(ctx, current, client.MergeFrom(orig)); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		return nil
	})
}

func (h *SelectionSyncHandler) clearDevicePool(ctx context.Context, name, pool string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha1.GPUDevice{}
		if err := h.client.Get(ctx, client.ObjectKey{Name: name}, current); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		if current.Annotations[assignmentAnnotation] == pool {
			return nil
		}
		if current.Status.PoolRef == nil || current.Status.PoolRef.Name != pool {
			return nil
		}
		orig := current.DeepCopy()
		current.Status.PoolRef = nil
		if current.Status.State == v1alpha1.GPUDeviceStateAssigned ||
			current.Status.State == v1alpha1.GPUDeviceStateReserved ||
			current.Status.State == v1alpha1.GPUDeviceStatePendingAssignment {
			current.Status.State = v1alpha1.GPUDeviceStateReady
		}
		if err := h.client.Status().Patch(ctx, current, client.MergeFrom(orig)); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		return nil
	})
}
