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

package service

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/conditions"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/eventrecord"
	invmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics/inventory"
)

type InventoryService struct {
	client   client.Client
	scheme   *runtime.Scheme
	recorder eventrecord.EventRecorderLogger
}

func NewInventoryService(c client.Client, scheme *runtime.Scheme, recorder eventrecord.EventRecorderLogger) *InventoryService {
	return &InventoryService{
		client:   c,
		scheme:   scheme,
		recorder: recorder,
	}
}

func (s *InventoryService) Reconcile(ctx context.Context, node *corev1.Node, snapshot invstate.NodeSnapshot, devices []*v1alpha1.GPUDevice) error {
	inventory := &v1alpha1.GPUNodeState{}
	inventory, err := commonobject.FetchObject(ctx, types.NamespacedName{Name: node.Name}, s.client, inventory)
	if err != nil {
		return err
	}
	if inventory == nil {
		if len(devices) == 0 {
			return nil
		}
		inventory = &v1alpha1.GPUNodeState{
			ObjectMeta: metav1.ObjectMeta{
				Name: node.Name,
			},
			Spec: v1alpha1.GPUNodeStateSpec{
				NodeName: node.Name,
			},
		}
		if err := controllerutil.SetOwnerReference(node, inventory, s.scheme); err != nil {
			return err
		}
		if err := s.client.Create(ctx, inventory); err != nil {
			return err
		}
	}

	specBefore := inventory.DeepCopy()
	changed := false

	if inventory.Spec.NodeName != node.Name {
		inventory.Spec.NodeName = node.Name
		changed = true
	}
	if err := controllerutil.SetOwnerReference(node, inventory, s.scheme); err != nil {
		return err
	}
	if !equality.Semantic.DeepEqual(specBefore.OwnerReferences, inventory.OwnerReferences) {
		changed = true
	}

	if changed {
		if err := s.client.Patch(ctx, inventory, client.MergeFrom(specBefore)); err != nil {
			return err
		}
		if err := s.client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory); err != nil {
			return err
		}
	}

	resource := reconciler.NewResource(
		types.NamespacedName{Name: node.Name},
		s.client,
		func() *v1alpha1.GPUNodeState { return &v1alpha1.GPUNodeState{} },
		func(obj *v1alpha1.GPUNodeState) v1alpha1.GPUNodeStateStatus { return obj.Status },
	)
	if err := resource.Fetch(ctx); err != nil {
		return err
	}
	if resource.IsEmpty() {
		return nil
	}

	inventory = resource.Changed()

	inventoryComplete := snapshot.FeatureDetected && len(snapshot.Devices) > 0
	inventoryReason := invstate.ReasonInventorySynced
	inventoryMessage := "inventory data collected"
	switch {
	case !snapshot.FeatureDetected:
		inventoryReason = invstate.ReasonNodeFeatureMissing
		inventoryMessage = "NodeFeature resource not discovered yet"
	case len(snapshot.Devices) == 0:
		inventoryReason = invstate.ReasonNoDevicesDiscovered
		inventoryMessage = "no NVIDIA devices detected on the node"
	}
	condBuilder := conditions.NewConditionBuilder(conditions.ConditionType(invstate.ConditionInventoryComplete)).
		Status(boolToConditionStatus(inventoryComplete)).
		Reason(conditions.CommonReason(inventoryReason)).
		Message(inventoryMessage).
		Generation(inventory.Generation)
	completeCond := condBuilder.Condition()
	prevComplete := conditions.FindStatusCondition(inventory.Status.Conditions, completeCond.Type)
	inventoryChanged := prevComplete == nil || prevComplete.Status != completeCond.Status || prevComplete.Reason != completeCond.Reason || prevComplete.Message != completeCond.Message
	conditions.SetCondition(condBuilder, &inventory.Status.Conditions)
	invmetrics.InventoryConditionSet(node.Name, invstate.ConditionInventoryComplete, inventoryComplete)

	if inventoryChanged && s.recorder != nil {
		eventType := corev1.EventTypeNormal
		if !inventoryComplete {
			eventType = corev1.EventTypeWarning
		}
		log := logr.FromContextOrDiscard(ctx).WithValues("node", node.Name)
		s.recorder.WithLogging(log).Eventf(
			node,
			eventType,
			invstate.EventInventoryChanged,
			"Condition %s changed to %t (%s)",
			invstate.ConditionInventoryComplete,
			inventoryComplete,
			inventoryReason,
		)
	}

	if equality.Semantic.DeepEqual(resource.Current().Status, inventory.Status) {
		return nil
	}

	return resource.Update(ctx)
}

func (s *InventoryService) UpdateDeviceMetrics(nodeName string, devices []*v1alpha1.GPUDevice) {
	updateDeviceStateMetrics(nodeName, devices)
	invmetrics.InventoryDevicesSet(nodeName, len(devices))
}

func boolToConditionStatus(value bool) metav1.ConditionStatus {
	if value {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func updateDeviceStateMetrics(nodeName string, devices []*v1alpha1.GPUDevice) {
	counts := make(map[string]int, len(devices))
	for _, device := range devices {
		stateKey := string(normalizeDeviceState(device.Status.State))
		counts[stateKey]++
	}
	seen := make(map[string]struct{}, len(counts))
	for state, count := range counts {
		invmetrics.InventoryDeviceStateSet(nodeName, state, count)
		seen[state] = struct{}{}
	}
	for _, state := range knownDeviceStates {
		key := string(state)
		if _, ok := seen[key]; !ok {
			invmetrics.InventoryDeviceStateDelete(nodeName, key)
		}
	}
}

func normalizeDeviceState(state v1alpha1.GPUDeviceState) v1alpha1.GPUDeviceState {
	if state == "" {
		return v1alpha1.GPUDeviceStateDiscovered
	}
	return state
}

var knownDeviceStates = []v1alpha1.GPUDeviceState{
	v1alpha1.GPUDeviceStateDiscovered,
	v1alpha1.GPUDeviceStateValidating,
	v1alpha1.GPUDeviceStateReady,
	v1alpha1.GPUDeviceStatePendingAssignment,
	v1alpha1.GPUDeviceStateAssigned,
	v1alpha1.GPUDeviceStateReserved,
	v1alpha1.GPUDeviceStateInUse,
	v1alpha1.GPUDeviceStateFaulted,
}
