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

package contracts

import "sigs.k8s.io/controller-runtime/pkg/reconcile"

// Result represents the outcome of a handler execution.
// It is an alias of controller-runtime reconcile.Result to align handler contracts with upstream patterns.
type Result = reconcile.Result

// MergeResult combines two handler results preferring the shortest requeue interval.
func MergeResult(current Result, next Result) Result {
	merged := current
	if next.Requeue {
		merged.Requeue = true
	}
	if next.RequeueAfter > 0 {
		if merged.RequeueAfter == 0 || next.RequeueAfter < merged.RequeueAfter {
			merged.RequeueAfter = next.RequeueAfter
		}
	}
	return merged
}
