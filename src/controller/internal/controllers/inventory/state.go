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
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

// inventoryState holds per-reconcile objects and helpers.
type inventoryState struct {
	node          *corev1.Node
	nodeFeature   *nfdv1alpha1.NodeFeature
	managedPolicy ManagedNodesPolicy
	snapshot      nodeSnapshot
}

func newInventoryState(node *corev1.Node, feature *nfdv1alpha1.NodeFeature, managed ManagedNodesPolicy) inventoryState {
	return inventoryState{
		node:          node,
		nodeFeature:   feature,
		managedPolicy: managed,
		snapshot:      buildNodeSnapshot(node, feature, managed),
	}
}

func (s inventoryState) Snapshot() nodeSnapshot {
	return s.snapshot
}

func (s inventoryState) AllowCleanup() bool {
	return s.snapshot.FeatureDetected || len(s.snapshot.Devices) > 0
}

func (s inventoryState) OrphanDevices(ctx context.Context, c client.Client) (map[string]struct{}, error) {
	existingDevices := &v1alpha1.GPUDeviceList{}
	if err := c.List(ctx, existingDevices, client.MatchingFields{deviceNodeIndexKey: s.node.Name}); err != nil {
		return nil, err
	}
	orphanDevices := make(map[string]struct{}, len(existingDevices.Items))
	for i := range existingDevices.Items {
		orphanDevices[existingDevices.Items[i].Name] = struct{}{}
	}
	return orphanDevices, nil
}

func (s inventoryState) CollectDetections(ctx context.Context, collect func(context.Context, string) (nodeDetection, error)) (nodeDetection, error) {
	if len(s.snapshot.Devices) == 0 {
		return nodeDetection{}, nil
	}
	return collect(ctx, s.node.Name)
}
