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

	candidates := k8sallocator.BuildCandidates(allocator.DefaultDriverName, slices, allocated)
	input := allocator.Input{
		Requests:   requests,
		Candidates: candidates,
	}
	result, err := a.allocator.Allocate(ctx, input)
	if err != nil || result == nil {
		return nil, err
	}

	return k8sallocator.BuildAllocationResult(claim, result, classes)
}

func listResourceSlices(ctx context.Context, cl client.Client) ([]resourcev1.ResourceSlice, error) {
	list := &resourcev1.ResourceSliceList{}
	if err := cl.List(ctx, list); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func listAllocatedDevices(ctx context.Context, cl client.Client) (map[allocator.DeviceKey]struct{}, error) {
	list := &resourcev1.ResourceClaimList{}
	if err := cl.List(ctx, list); err != nil {
		return nil, err
	}

	allocated := map[allocator.DeviceKey]struct{}{}
	for i := range list.Items {
		claim := &list.Items[i]
		if claim.Status.Allocation == nil {
			continue
		}
		for _, res := range claim.Status.Allocation.Devices.Results {
			key := allocator.DeviceKey{Driver: res.Driver, Pool: res.Pool, Device: res.Device}
			allocated[key] = struct{}{}
		}
	}
	return allocated, nil
}
