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
	"strings"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

const (
	namespacedPoolResourcePrefix = "gpu.deckhouse.io"
	clusterPoolResourcePrefix    = "cluster.gpu.deckhouse.io"

	deviceIgnoreKey = "gpu.deckhouse.io/ignore"

	namespacedAssignmentAnnotation = "gpu.deckhouse.io/assignment"
	clusterAssignmentAnnotation    = "cluster.gpu.deckhouse.io/assignment"
	assignmentAnnotation           = namespacedAssignmentAnnotation
)

func isClusterPool(pool *v1alpha1.GPUPool) bool {
	return pool != nil && pool.Kind == "ClusterGPUPool"
}

func poolResourcePrefixFor(pool *v1alpha1.GPUPool) string {
	if isClusterPool(pool) {
		return clusterPoolResourcePrefix
	}
	return namespacedPoolResourcePrefix
}

func alternatePoolResourcePrefixFor(pool *v1alpha1.GPUPool) string {
	if isClusterPool(pool) {
		return namespacedPoolResourcePrefix
	}
	return clusterPoolResourcePrefix
}

func assignmentAnnotationKey(pool *v1alpha1.GPUPool) string {
	if isClusterPool(pool) {
		return clusterAssignmentAnnotation
	}
	return namespacedAssignmentAnnotation
}

func isDeviceIgnored(dev *v1alpha1.GPUDevice) bool {
	if dev == nil {
		return false
	}
	return strings.EqualFold(dev.Labels[deviceIgnoreKey], "true")
}

func poolRefMatchesPool(pool *v1alpha1.GPUPool, ref *v1alpha1.GPUPoolReference) bool {
	if pool == nil || ref == nil {
		return false
	}
	if ref.Name != pool.Name {
		return false
	}

	// Cluster pools must not carry namespace in poolRef.
	if isClusterPool(pool) || pool.Namespace == "" {
		return strings.TrimSpace(ref.Namespace) == ""
	}

	// Backward compatibility: legacy poolRef without namespace still matches.
	if strings.TrimSpace(ref.Namespace) == "" {
		return true
	}
	return ref.Namespace == pool.Namespace
}
