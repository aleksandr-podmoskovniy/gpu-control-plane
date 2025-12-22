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
	"context"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func (r *Reconciler) patchStatus(ctx context.Context, key types.NamespacedName, used int32) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha1.GPUPool{}
		if err := r.client.Get(ctx, key, current); err != nil {
			return client.IgnoreNotFound(err)
		}
		original := current.DeepCopy()

		total := current.Status.Capacity.Total
		available := total - used
		if available < 0 {
			available = 0
		}

		if current.Status.Capacity.Used == used && current.Status.Capacity.Available == available {
			return nil
		}

		current.Status.Capacity.Used = used
		current.Status.Capacity.Available = available
		return r.client.Status().Patch(ctx, current, client.MergeFrom(original))
	})
}
