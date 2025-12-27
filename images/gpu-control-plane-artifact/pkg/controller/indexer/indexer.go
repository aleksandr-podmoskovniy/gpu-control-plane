/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package indexer

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonannotations "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/annotations"
)

const (
	// GPUDeviceNodeField indexes GPUDevice by status.nodeName for fast lookups.
	GPUDeviceNodeField = "status.nodeName"
	// GPUDevicePoolRefNameField indexes GPUDevice by status.poolRef.name for pool-specific queries.
	GPUDevicePoolRefNameField = "status.poolRef.name"
	// GPUDeviceNamespacedAssignmentField indexes GPUDevice by gpu.deckhouse.io/assignment annotation value.
	GPUDeviceNamespacedAssignmentField = "metadata.annotations." + commonannotations.GPUDeviceAssignment
	// GPUDeviceClusterAssignmentField indexes GPUDevice by cluster.gpu.deckhouse.io/assignment annotation value.
	GPUDeviceClusterAssignmentField = "metadata.annotations." + commonannotations.ClusterGPUDeviceAssignment
	// GPUPoolNameField indexes namespaced GPUPools by metadata.name (cluster-unique by policy).
	GPUPoolNameField = "metadata.name"
	// NodeTaintKeyField indexes Nodes by spec.taints.key for taint cleanup operations.
	NodeTaintKeyField = "spec.taints.key"
)

type IndexGetter func() (obj client.Object, field string, extractValue client.IndexerFunc)

var IndexGetters = []IndexGetter{
	IndexGPUDeviceByNode,
	IndexGPUDeviceByPoolRefName,
	IndexGPUDeviceByNamespacedAssignment,
	IndexGPUDeviceByClusterAssignment,
	IndexGPUPoolByName,
	IndexNodeByTaintKey,
}
