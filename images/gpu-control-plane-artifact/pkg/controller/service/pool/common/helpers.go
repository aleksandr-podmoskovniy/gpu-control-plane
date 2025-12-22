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

package common

import (
	"fmt"
	"strings"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func isClusterPool(pool *v1alpha1.GPUPool) bool {
	return pool != nil && pool.Kind == "ClusterGPUPool"
}

func PoolResourcePrefixFor(pool *v1alpha1.GPUPool) string {
	if isClusterPool(pool) {
		return ClusterPoolResourcePrefix
	}
	return NamespacedPoolResourcePrefix
}

func AssignmentAnnotationKey(pool *v1alpha1.GPUPool) string {
	if isClusterPool(pool) {
		return ClusterAssignmentAnnotation
	}
	return NamespacedAssignmentAnnotation
}

func PoolRefMatchesPool(pool *v1alpha1.GPUPool, ref *v1alpha1.GPUPoolReference) bool {
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

	return ref.Namespace == pool.Namespace
}

func PoolLabelKey(pool *v1alpha1.GPUPool) string {
	return fmt.Sprintf("%s/%s", PoolResourcePrefixFor(pool), pool.Name)
}

func PoolValueFromKey(key string) string {
	parts := strings.Split(key, "/")
	return parts[len(parts)-1]
}
