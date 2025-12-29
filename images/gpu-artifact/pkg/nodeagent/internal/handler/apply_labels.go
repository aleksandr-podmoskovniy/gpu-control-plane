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

package handler

import (
	"context"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

func (h *ApplyHandler) ensureLabels(ctx context.Context, obj *gpuv1alpha1.PhysicalGPU, nodeName string, dev state.Device) error {
	desired := state.LabelsForDevice(nodeName, dev)
	labels := obj.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	changed := false
	for key, value := range desired {
		if labels[key] != value {
			labels[key] = value
			changed = true
		}
	}

	if !changed {
		return nil
	}

	patchBase := obj.DeepCopy()
	obj.Labels = labels
	return h.store.Patch(ctx, obj, patchBase)
}
