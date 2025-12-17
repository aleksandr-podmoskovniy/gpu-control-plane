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

package inventory

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/conditions"
	cpmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
)

type InventoryService interface {
	Reconcile(ctx context.Context, node *corev1.Node, snapshot nodeSnapshot, devices []*v1alpha1.GPUDevice) error
	UpdateDeviceMetrics(nodeName string, devices []*v1alpha1.GPUDevice)
}

type inventoryService struct {
	client   client.Client
	scheme   *runtimeScheme
	recorder eventRecorder
}

func newInventoryService(c client.Client, scheme *runtimeScheme, recorder eventRecorder) InventoryService {
	return &inventoryService{
		client:   c,
		scheme:   scheme,
		recorder: recorder,
	}
}

func (s *inventoryService) Reconcile(ctx context.Context, node *corev1.Node, snapshot nodeSnapshot, devices []*v1alpha1.GPUDevice) error {
	inventory := &v1alpha1.GPUNodeState{}
	err := s.client.Get(ctx, types.NamespacedName{Name: node.Name}, inventory)
	if apierrors.IsNotFound(err) {
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
	} else if err != nil {
		return err
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

	resource := reconciler.NewResource(inventory, s.client)
	condBuilder := conditions.New(&inventory.Status.Conditions)

	inventoryComplete := snapshot.FeatureDetected && len(snapshot.Devices) > 0
	inventoryReason := reasonInventorySynced
	inventoryMessage := "inventory data collected"
	switch {
	case !snapshot.FeatureDetected:
		inventoryReason = reasonNodeFeatureMissing
		inventoryMessage = "NodeFeature resource not discovered yet"
	case len(snapshot.Devices) == 0:
		inventoryReason = reasonNoDevicesDiscovered
		inventoryMessage = "no NVIDIA devices detected on the node"
	}
	completeCond := metav1.Condition{
		Type:               conditionInventoryComplete,
		Status:             boolToConditionStatus(inventoryComplete),
		Reason:             inventoryReason,
		Message:            inventoryMessage,
		ObservedGeneration: inventory.Generation,
	}
	prevComplete := condBuilder.Find(conditionInventoryComplete)
	inventoryChanged := prevComplete == nil || prevComplete.Status != completeCond.Status || prevComplete.Reason != completeCond.Reason || prevComplete.Message != completeCond.Message
	condBuilder.Set(completeCond)
	cpmetrics.InventoryConditionSet(node.Name, conditionInventoryComplete, inventoryComplete)

	if inventoryChanged {
		eventType := corev1.EventTypeNormal
		if !inventoryComplete {
			eventType = corev1.EventTypeWarning
		}
		s.recorder.Eventf(node, eventType, eventInventoryChanged, "Condition %s changed to %t (%s)", conditionInventoryComplete, inventoryComplete, inventoryReason)
	}

	if equality.Semantic.DeepEqual(resource.Original().Status, inventory.Status) {
		return nil
	}

	return resource.PatchStatus(ctx)
}

func (s *inventoryService) UpdateDeviceMetrics(nodeName string, devices []*v1alpha1.GPUDevice) {
	updateDeviceStateMetrics(nodeName, devices)
	cpmetrics.InventoryDevicesSet(nodeName, len(devices))
}
