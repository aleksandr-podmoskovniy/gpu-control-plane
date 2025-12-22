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

const (
	// PoolNameKey identifies the pool referenced by a workload Pod that requests GPU resources.
	PoolNameKey = "gpu-control-plane.deckhouse.io/pool"
	// PoolScopeKey distinguishes between namespaced and cluster-scoped pools.
	PoolScopeKey = "gpu-control-plane.deckhouse.io/pool-scope"

	PoolScopeNamespaced = "namespaced"
	PoolScopeCluster    = "cluster"
)
