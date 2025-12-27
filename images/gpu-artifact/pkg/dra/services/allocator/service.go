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

package allocator

import (
	"context"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
)

// Service performs allocation flow orchestration.
type Service struct {
	inventory ports.InventoryProvider
	writer    ports.AllocationWriter
}

// NewService creates a new allocator service.
func NewService(inventory ports.InventoryProvider, writer ports.AllocationWriter) *Service {
	return &Service{
		inventory: inventory,
		writer:    writer,
	}
}

// RunOnce executes a single allocation cycle (no-op for now).
func (s *Service) RunOnce(ctx context.Context) error {
	snapshot, err := s.inventory.Snapshot(ctx)
	if err != nil {
		return err
	}

	result := domain.AllocationResult{
		NodeName: snapshot.NodeName,
	}
	return s.writer.Write(ctx, result)
}
