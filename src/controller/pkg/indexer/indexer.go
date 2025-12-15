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

package indexer

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

const (
	// GPUDeviceNodeField indexes GPUDevice by status.nodeName for fast lookups.
	GPUDeviceNodeField = "status.nodeName"
	// GPUDevicePoolRefNameField indexes GPUDevice by status.poolRef.name for pool-specific queries.
	GPUDevicePoolRefNameField = "status.poolRef.name"
	// GPUDeviceNamespacedAssignmentField indexes GPUDevice by gpu.deckhouse.io/assignment annotation value.
	GPUDeviceNamespacedAssignmentField = "metadata.annotations.gpu.deckhouse.io/assignment"
	// GPUDeviceClusterAssignmentField indexes GPUDevice by cluster.gpu.deckhouse.io/assignment annotation value.
	GPUDeviceClusterAssignmentField = "metadata.annotations.cluster.gpu.deckhouse.io/assignment"
	// GPUPoolNameField indexes namespaced GPUPools by metadata.name (cluster-unique by policy).
	GPUPoolNameField = "metadata.name"
	// NodeTaintKeyField indexes Nodes by spec.taints.key for taint cleanup operations.
	NodeTaintKeyField = "spec.taints.key"
)

// IndexGPUDeviceByNode registers a field indexer that maps GPUDevices to their node names.
func IndexGPUDeviceByNode(ctx context.Context, idx client.FieldIndexer) error {
	if idx == nil {
		return nil
	}
	return idx.IndexField(ctx, &v1alpha1.GPUDevice{}, GPUDeviceNodeField, func(obj client.Object) []string {
		dev, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || dev.Status.NodeName == "" {
			return nil
		}
		return []string{dev.Status.NodeName}
	})
}

// IndexGPUDeviceByPoolRefName registers a field indexer that maps GPUDevices to their poolRef names.
func IndexGPUDeviceByPoolRefName(ctx context.Context, idx client.FieldIndexer) error {
	if idx == nil {
		return nil
	}
	return idx.IndexField(ctx, &v1alpha1.GPUDevice{}, GPUDevicePoolRefNameField, func(obj client.Object) []string {
		dev, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || dev.Status.PoolRef == nil || dev.Status.PoolRef.Name == "" {
			return nil
		}
		return []string{dev.Status.PoolRef.Name}
	})
}

// IndexGPUDeviceByNamespacedAssignment registers a field indexer that maps GPUDevices to namespaced assignment annotation values.
func IndexGPUDeviceByNamespacedAssignment(ctx context.Context, idx client.FieldIndexer) error {
	if idx == nil {
		return nil
	}
	return idx.IndexField(ctx, &v1alpha1.GPUDevice{}, GPUDeviceNamespacedAssignmentField, func(obj client.Object) []string {
		dev, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || dev.Annotations == nil {
			return nil
		}
		if value := dev.Annotations["gpu.deckhouse.io/assignment"]; value != "" {
			return []string{value}
		}
		return nil
	})
}

// IndexGPUDeviceByClusterAssignment registers a field indexer that maps GPUDevices to cluster assignment annotation values.
func IndexGPUDeviceByClusterAssignment(ctx context.Context, idx client.FieldIndexer) error {
	if idx == nil {
		return nil
	}
	return idx.IndexField(ctx, &v1alpha1.GPUDevice{}, GPUDeviceClusterAssignmentField, func(obj client.Object) []string {
		dev, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || dev.Annotations == nil {
			return nil
		}
		if value := dev.Annotations["cluster.gpu.deckhouse.io/assignment"]; value != "" {
			return []string{value}
		}
		return nil
	})
}

// IndexGPUPoolByName registers a field indexer that maps namespaced GPUPools to their names.
func IndexGPUPoolByName(ctx context.Context, idx client.FieldIndexer) error {
	if idx == nil {
		return nil
	}
	return idx.IndexField(ctx, &v1alpha1.GPUPool{}, GPUPoolNameField, func(obj client.Object) []string {
		pool, ok := obj.(*v1alpha1.GPUPool)
		if !ok || pool.Name == "" {
			return nil
		}
		return []string{pool.Name}
	})
}

// IndexNodeByTaintKey registers a field indexer that maps Nodes to taint keys present on them.
func IndexNodeByTaintKey(ctx context.Context, idx client.FieldIndexer) error {
	if idx == nil {
		return nil
	}
	return idx.IndexField(ctx, &corev1.Node{}, NodeTaintKeyField, func(obj client.Object) []string {
		node, ok := obj.(*corev1.Node)
		if !ok || len(node.Spec.Taints) == 0 {
			return nil
		}
		seen := make(map[string]struct{}, len(node.Spec.Taints))
		keys := make([]string, 0, len(node.Spec.Taints))
		for _, taint := range node.Spec.Taints {
			if taint.Key == "" {
				continue
			}
			if _, ok := seen[taint.Key]; ok {
				continue
			}
			seen[taint.Key] = struct{}{}
			keys = append(keys, taint.Key)
		}
		if len(keys) == 0 {
			return nil
		}
		return keys
	})
}
