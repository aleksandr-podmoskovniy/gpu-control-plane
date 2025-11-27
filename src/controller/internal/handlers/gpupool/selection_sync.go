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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

	assigned := make(map[string]v1alpha1.GPUDevice)
	for _, dev := range devices.Items {
		if dev.Annotations[assignmentAnnotation] != pool.Name {
			continue
		}
		assigned[dev.Status.InventoryID] = dev
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
		baseUnits   int32
		devStatuses []v1alpha1.GPUPoolDeviceStatus
		nodeTotals  = map[string]int32{}
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
		candidates := FilterDevices(inv.Status.Hardware.Devices, pool.Spec.DeviceSelector)
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
			dev.AutoAttach = devCR.Status.AutoAttach
			devStatuses = append(devStatuses, v1alpha1.GPUPoolDeviceStatus{
				InventoryID: dev.InventoryID,
				Node:        inv.Name,
				State:       dev.State,
				AutoAttach:  dev.AutoAttach,
			})
			nodeTotals[inv.Name]++
			if dev.State == v1alpha1.GPUDeviceStateReady {
				if pool.Spec.Resource.MaxDevicesPerNode != nil && takenOnNode >= *pool.Spec.Resource.MaxDevicesPerNode {
					continue
				}
				units, base := h.unitsForDevice(dev, pool)
				if units > 0 {
					totalUnits += units
					baseUnits += base
					takenOnNode++
				}
			}
		}
	}

	pool.Status.Devices = devStatuses
	pool.Status.Capacity.Total = totalUnits
	if totalUnits >= pool.Status.Capacity.Used {
		pool.Status.Capacity.Available = totalUnits - pool.Status.Capacity.Used
	}
	pool.Status.Capacity.Unit = pool.Spec.Resource.Unit
	pool.Status.Capacity.BaseUnits = baseUnits
	pool.Status.Capacity.SlicesPerUnit = pool.Spec.Resource.SlicesPerUnit

	pool.Status.Nodes = make([]v1alpha1.GPUPoolNodeStatus, 0, len(nodeTotals))
	for node, total := range nodeTotals {
		pool.Status.Nodes = append(pool.Status.Nodes, v1alpha1.GPUPoolNodeStatus{
			Name:            node,
			TotalDevices:    total,
			AssignedDevices: 0,
		})
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
