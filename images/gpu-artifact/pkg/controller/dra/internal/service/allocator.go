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

	resourcev1 "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8sallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/k8s/allocator"
	allocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

// Allocator loads claim inputs and computes an allocation result.
type Allocator struct {
	client    client.Client
	allocator *allocator.Service
	classes   *DeviceClassService
}

// NewAllocator creates an Allocator service.
func NewAllocator(client client.Client) *Allocator {
	return &Allocator{
		client:    client,
		allocator: allocator.NewService(),
		classes:   NewDeviceClassService(client),
	}
}

// Allocate computes allocation for the provided claim.
func (a *Allocator) Allocate(ctx context.Context, claim *resourcev1.ResourceClaim) (*resourcev1.AllocationResult, error) {
	if claim == nil {
		return nil, nil
	}

	classes, err := a.classes.Load(ctx, claim)
	if err != nil {
		return nil, err
	}

	requests, err := k8sallocator.BuildRequests(claim, classes)
	if err != nil {
		return nil, err
	}
	if len(requests) == 0 {
		return nil, nil
	}

	slices, err := listResourceSlices(ctx, a.client)
	if err != nil {
		return nil, err
	}

	allocated, err := listAllocatedDevices(ctx, a.client)
	if err != nil {
		return nil, err
	}

	candidates := k8sallocator.BuildCandidates(allocator.DefaultDriverName, slices)
	counterSets := k8sallocator.BuildCounterSets(allocator.DefaultDriverName, slices)
	input := allocator.Input{
		Requests:    requests,
		Candidates:  candidates,
		Allocated:   allocated,
		CounterSets: counterSets,
	}
	result, err := a.allocator.Allocate(ctx, input)
	if err != nil || result == nil {
		return nil, err
	}

	return k8sallocator.BuildAllocationResult(claim, result, classes)
}
