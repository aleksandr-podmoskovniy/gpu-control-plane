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
	corev1 "k8s.io/api/core/v1"

	invconsts "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/consts"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

type nodeSnapshot = invstate.NodeSnapshot
type deviceSnapshot = invstate.DeviceSnapshot

type ManagedNodesPolicy = invstate.ManagedNodesPolicy
type DeviceApprovalPolicy = invstate.DeviceApprovalPolicy

const (
	deviceNodeLabelKey  = invconsts.DeviceNodeLabelKey
	deviceIndexLabelKey = invconsts.DeviceIndexLabelKey

	conditionInventoryComplete = invconsts.ConditionInventoryComplete
	reasonInventorySynced      = invconsts.ReasonInventorySynced
	reasonNoDevicesDiscovered  = invconsts.ReasonNoDevicesDiscovered
	reasonNodeFeatureMissing   = invconsts.ReasonNodeFeatureMissing

	nodeFeatureNodeNameLabel = invconsts.NodeFeatureNodeNameLabel

	defaultManagedNodeLabelKey = invconsts.DefaultManagedNodeLabelKey
)

func buildNodeSnapshot(node *corev1.Node, feature *nfdv1alpha1.NodeFeature, policy ManagedNodesPolicy) nodeSnapshot {
	return invstate.BuildNodeSnapshot(node, feature, policy)
}

func buildDeviceName(nodeName string, info deviceSnapshot) string {
	return invstate.BuildDeviceName(nodeName, info)
}

func buildInventoryID(nodeName string, info deviceSnapshot) string {
	return invstate.BuildInventoryID(nodeName, info)
}
