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
	"sort"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

const DefaultDriverName = "gpu.deckhouse.io"

// Service performs allocation on a prepared inventory snapshot.
type Service struct{}

// NewService creates a new allocator service.
func NewService() *Service {
	return &Service{}
}

// Allocate computes an allocation result for the given claim.
func (s *Service) Allocate(ctx context.Context, input Input) (*domain.AllocationResult, error) {
	if len(input.Requests) == 0 {
		return nil, nil
	}
	if len(input.Candidates) == 0 {
		return nil, nil
	}

	nodes := groupByNode(input.Candidates)
	if len(nodes) == 0 {
		return nil, nil
	}

	nodeNames := make([]string, 0, len(nodes))
	for nodeName := range nodes {
		nodeNames = append(nodeNames, nodeName)
	}
	sort.Strings(nodeNames)

	for _, nodeName := range nodeNames {
		alloc, ok, err := allocateOnNode(ctx, nodeName, nodes[nodeName], input.Requests)
		if err != nil {
			return nil, err
		}
		if ok {
			return alloc, nil
		}
	}

	return nil, nil
}
