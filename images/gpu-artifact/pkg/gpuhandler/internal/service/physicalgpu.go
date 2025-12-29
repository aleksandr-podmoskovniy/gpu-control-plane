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

package service

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
)

// PhysicalGPUService queries PhysicalGPU objects.
type PhysicalGPUService struct {
	client client.Client
}

// NewPhysicalGPUService constructs a PhysicalGPU service.
func NewPhysicalGPUService(client client.Client) *PhysicalGPUService {
	return &PhysicalGPUService{client: client}
}

// ListByNode returns PhysicalGPU objects for the given node.
func (s *PhysicalGPUService) ListByNode(ctx context.Context, nodeName string) ([]gpuv1alpha1.PhysicalGPU, error) {
	list := &gpuv1alpha1.PhysicalGPUList{}
	if err := s.client.List(ctx, list, client.MatchingLabels{state.LabelNode: nodeName}); err != nil {
		return nil, err
	}
	return list.Items, nil
}

// PatchStatus patches PhysicalGPU status using the provided base object.
func (s *PhysicalGPUService) PatchStatus(ctx context.Context, obj, base *gpuv1alpha1.PhysicalGPU) error {
	if base == nil {
		base = obj.DeepCopy()
	}
	return s.client.Status().Patch(ctx, obj, client.MergeFrom(base))
}
