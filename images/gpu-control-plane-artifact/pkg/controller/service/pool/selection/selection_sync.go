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

package selection

import (
	"context"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

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

func (h *SelectionSyncHandler) HandlePool(ctx context.Context, pool *v1alpha1.GPUPool) (reconcile.Result, error) {
	assignmentKey := poolcommon.AssignmentAnnotationKey(pool)
	assignmentField := indexer.GPUDeviceNamespacedAssignmentField
	if assignmentKey == poolcommon.ClusterAssignmentAnnotation {
		assignmentField = indexer.GPUDeviceClusterAssignmentField
	}

	assignedDevices := &v1alpha1.GPUDeviceList{}
	if err := h.client.List(ctx, assignedDevices, client.MatchingFields{assignmentField: pool.Name}); err != nil {
		return reconcile.Result{}, err
	}
	poolRefDevices := &v1alpha1.GPUDeviceList{}
	if err := h.client.List(ctx, poolRefDevices, client.MatchingFields{indexer.GPUDevicePoolRefNameField: pool.Name}); err != nil {
		return reconcile.Result{}, err
	}

	// Nodes allowed by pool.Spec.NodeSelector (if set).
	var nodeSelector labels.Selector
	if pool.Spec.NodeSelector != nil {
		var err error
		nodeSelector, err = metav1.LabelSelectorAsSelector(pool.Spec.NodeSelector)
		if err != nil {
			return reconcile.Result{}, apierrors.NewBadRequest("invalid nodeSelector")
		}
	}
	// Collect devices explicitly assigned to this pool via annotation and matching selectors.
	assigned := make([]v1alpha1.GPUDevice, 0)
	for i := range assignedDevices.Items {
		dev := assignedDevices.Items[i]
		if poolcommon.IsDeviceIgnored(&dev) {
			continue
		}

		nodeName := poolcommon.DeviceNodeName(&dev)
		if nodeName == "" {
			continue
		}
		assigned = append(assigned, dev)
	}
	assigned = poolcommon.FilterDevices(assigned, pool.Spec.DeviceSelector)

	// Group by node and sort deterministically to apply maxDevicesPerNode.
	byNode := map[string][]v1alpha1.GPUDevice{}
	for _, dev := range assigned {
		nodeName := poolcommon.DeviceNodeName(&dev)
		byNode[nodeName] = append(byNode[nodeName], dev)
	}
	for node := range byNode {
		sort.Slice(byNode[node], func(i, j int) bool {
			return deviceSortKey(byNode[node][i]) < deviceSortKey(byNode[node][j])
		})
	}

	eligibleNodes := map[string]struct{}{}
	if nodeSelector != nil {
		for nodeName := range byNode {
			node := &corev1.Node{}
			node, err := commonobject.FetchObject(ctx, client.ObjectKey{Name: nodeName}, h.client, node)
			if err != nil {
				return reconcile.Result{}, err
			}
			if node == nil {
				continue
			}
			if nodeSelector.Matches(labels.Set(node.Labels)) {
				eligibleNodes[nodeName] = struct{}{}
			}
		}
		for nodeName := range byNode {
			if _, ok := eligibleNodes[nodeName]; !ok {
				delete(byNode, nodeName)
			}
		}
	}

	var (
		totalUnits int32
		toUpdate   []v1alpha1.GPUDevice
	)

	for _, devs := range byNode {
		var takenOnNode int32
		for _, dev := range devs {
			if needsAssignmentUpdate(dev, pool.Name, pool.Namespace) {
				toUpdate = append(toUpdate, dev)
			}

			// Pool capacity is a static upper bound derived from assignment annotations,
			// not a real-time availability signal. Runtime readiness (validator/device-plugin)
			// is tracked separately via device states and pool conditions.
			if pool.Spec.Resource.MaxDevicesPerNode != nil && takenOnNode >= *pool.Spec.Resource.MaxDevicesPerNode {
				continue
			}
			units := h.unitsForDevice(dev, pool)
			if units <= 0 {
				continue
			}
			totalUnits += units
			takenOnNode++
		}
	}

	// Unassign devices that still point to this pool but no longer carry the assignment annotation.
	for i := range poolRefDevices.Items {
		dev := &poolRefDevices.Items[i]
		if dev.Annotations[assignmentKey] == pool.Name {
			continue
		}
		if !poolcommon.PoolRefMatchesPool(pool, dev.Status.PoolRef) {
			continue
		}
		if err := h.clearDevicePool(ctx, dev.Name, pool.Name, pool.Namespace, assignmentKey); err != nil {
			return reconcile.Result{}, err
		}
	}

	pool.Status.Capacity.Total = totalUnits

	for i := range toUpdate {
		dev := toUpdate[i]
		if err := h.assignDeviceWithRetry(ctx, dev.Name, pool.Name, pool.Namespace); err != nil {
			return reconcile.Result{}, err
		}
	}

	h.log.V(2).Info("synchronised pool selection", "pool", pool.Name, "assignedDevices", len(assigned), "capacity", totalUnits)
	return reconcile.Result{}, nil
}

func deviceSortKey(dev v1alpha1.GPUDevice) string {
	key := strings.TrimSpace(dev.Status.InventoryID)
	if key != "" {
		return key
	}
	return dev.Name
}

func (h *SelectionSyncHandler) unitsForDevice(dev v1alpha1.GPUDevice, pool *v1alpha1.GPUPool) int32 {
	if pool.Spec.Resource.Unit == "MIG" {
		if pool.Spec.Resource.MIGProfile == "" {
			return 0
		}
		var profileCount int32
		for _, t := range dev.Status.Hardware.MIG.Types {
			if t.Name == pool.Spec.Resource.MIGProfile {
				profileCount += t.Count
			}
		}
		if profileCount == 0 {
			return 0
		}
		if pool.Spec.Resource.SlicesPerUnit > 0 {
			return profileCount * pool.Spec.Resource.SlicesPerUnit
		}
		return profileCount
	}
	if pool.Spec.Resource.SlicesPerUnit > 0 {
		return pool.Spec.Resource.SlicesPerUnit
	}
	return 1
}

func needsAssignmentUpdate(dev v1alpha1.GPUDevice, poolName, poolNamespace string) bool {
	ref := dev.Status.PoolRef
	if ref == nil || ref.Name != poolName {
		return true
	}
	if strings.TrimSpace(poolNamespace) == "" {
		if strings.TrimSpace(ref.Namespace) != "" {
			return true
		}
	} else if strings.TrimSpace(ref.Namespace) != poolNamespace {
		return true
	}
	if dev.Status.State == v1alpha1.GPUDeviceStateReady {
		return true
	}
	return false
}

func (h *SelectionSyncHandler) assignDeviceWithRetry(ctx context.Context, name, poolName, poolNamespace string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha1.GPUDevice{}
		current, err := commonobject.FetchObject(ctx, client.ObjectKey{Name: name}, h.client, current)
		if err != nil {
			return err
		}
		if current == nil {
			return nil
		}
		orig := current.DeepCopy()
		ref := &v1alpha1.GPUPoolReference{Name: poolName}
		if strings.TrimSpace(poolNamespace) != "" {
			ref.Namespace = poolNamespace
		}
		current.Status.PoolRef = ref
		// Do not transition to Assigned without DP validator: Ready -> PendingAssignment.
		if current.Status.State == v1alpha1.GPUDeviceStateReady {
			current.Status.State = v1alpha1.GPUDeviceStatePendingAssignment
		}
		if err := h.client.Status().Patch(ctx, current, client.MergeFrom(orig)); err != nil {
			return client.IgnoreNotFound(err)
		}
		return nil
	})
}

func (h *SelectionSyncHandler) clearDevicePool(ctx context.Context, name, poolName, poolNamespace, assignmentKey string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha1.GPUDevice{}
		current, err := commonobject.FetchObject(ctx, client.ObjectKey{Name: name}, h.client, current)
		if err != nil {
			return err
		}
		if current == nil {
			return nil
		}
		if current.Annotations[assignmentKey] == poolName {
			return nil
		}
		ref := current.Status.PoolRef
		if ref == nil || ref.Name != poolName {
			return nil
		}
		if strings.TrimSpace(poolNamespace) == "" {
			if strings.TrimSpace(ref.Namespace) != "" {
				return nil
			}
		} else if strings.TrimSpace(ref.Namespace) != "" && ref.Namespace != poolNamespace {
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
			return client.IgnoreNotFound(err)
		}
		return nil
	})
}
