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
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

// Store abstracts CRUD operations for PhysicalGPU objects.
type Store interface {
	Get(ctx context.Context, name string) (*gpuv1alpha1.PhysicalGPU, error)
	Create(ctx context.Context, obj *gpuv1alpha1.PhysicalGPU) error
	Patch(ctx context.Context, obj *gpuv1alpha1.PhysicalGPU, base *gpuv1alpha1.PhysicalGPU) error
	PatchStatus(ctx context.Context, obj *gpuv1alpha1.PhysicalGPU, base *gpuv1alpha1.PhysicalGPU) error
	ListByNode(ctx context.Context, nodeName string) ([]gpuv1alpha1.PhysicalGPU, error)
	Delete(ctx context.Context, obj *gpuv1alpha1.PhysicalGPU) error
}

// ClientStore uses a controller-runtime client for storage operations.
type ClientStore struct {
	client client.Client
}

// NewClientStore creates a store backed by a controller-runtime client.
func NewClientStore(client client.Client) *ClientStore {
	return &ClientStore{client: client}
}

// Get fetches a PhysicalGPU by name.
func (s *ClientStore) Get(ctx context.Context, name string) (*gpuv1alpha1.PhysicalGPU, error) {
	obj := &gpuv1alpha1.PhysicalGPU{}
	if err := s.client.Get(ctx, client.ObjectKey{Name: name}, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// Create adds a new PhysicalGPU.
func (s *ClientStore) Create(ctx context.Context, obj *gpuv1alpha1.PhysicalGPU) error {
	return s.client.Create(ctx, obj)
}

// Patch updates metadata fields.
func (s *ClientStore) Patch(ctx context.Context, obj *gpuv1alpha1.PhysicalGPU, base *gpuv1alpha1.PhysicalGPU) error {
	return s.client.Patch(ctx, obj, client.MergeFrom(base))
}

// PatchStatus updates the status subresource.
func (s *ClientStore) PatchStatus(ctx context.Context, obj *gpuv1alpha1.PhysicalGPU, base *gpuv1alpha1.PhysicalGPU) error {
	return s.client.Status().Patch(ctx, obj, client.MergeFrom(base))
}

// ListByNode returns PhysicalGPU objects on a node.
func (s *ClientStore) ListByNode(ctx context.Context, nodeName string) ([]gpuv1alpha1.PhysicalGPU, error) {
	list := &gpuv1alpha1.PhysicalGPUList{}
	if err := s.client.List(ctx, list, client.MatchingLabels{state.LabelNode: nodeName}); err != nil {
		return nil, err
	}
	return list.Items, nil
}

// Delete removes a PhysicalGPU object.
func (s *ClientStore) Delete(ctx context.Context, obj *gpuv1alpha1.PhysicalGPU) error {
	return s.client.Delete(ctx, obj)
}
