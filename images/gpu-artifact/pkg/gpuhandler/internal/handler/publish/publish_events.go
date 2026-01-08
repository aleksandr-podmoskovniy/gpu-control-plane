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

package publish

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/dynamic-resource-allocation/resourceslice"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

const (
	reasonResourceSlicePublished     = "ResourceSlicePublished"
	reasonResourceSlicePublishFailed = "ResourceSlicePublishFailed"
	reasonMigPlacementMismatch       = "MigPlacementMismatch"
)

func (h *PublishResourcesHandler) recordPublish(ctx context.Context, st state.State, resources resourceslice.DriverResources, err error) {
	if h.recorder == nil {
		return
	}
	ready := st.Ready()
	if len(ready) == 0 {
		return
	}

	eventType := corev1.EventTypeNormal
	reason := reasonResourceSlicePublished
	msg := "resource slices published"
	if err != nil {
		eventType = corev1.EventTypeWarning
		reason = reasonResourceSlicePublishFailed
		msg = fmt.Sprintf("resource slice publish failed: %v", err)
	}

	log := logger.FromContext(ctx).With("node", st.NodeName())
	for poolName, pool := range resources.Pools {
		for i, slice := range pool.Slices {
			args := []any{
				"reason", reason,
				"pool", poolName,
				"sliceIndex", i,
				"offerCount", len(slice.Devices),
			}
			if err != nil {
				args = append(args, logger.SlogErr(err))
			}
			log.Info("resource slice publish status", args...)
		}
	}

	for i := range ready {
		h.recorder.WithLogging(log).Event(&ready[i], eventType, reason, msg)
	}
}
