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
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8sallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/k8s/allocator"
	allocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

// Allocator loads claim inputs and computes an allocation result.
type Allocator struct {
	client    client.Client
	allocator *allocator.Service
}

// NewAllocator creates an Allocator service.
func NewAllocator(client client.Client) *Allocator {
	return &Allocator{
		client:    client,
		allocator: allocator.NewService(),
	}
}

// Allocate computes allocation for the provided claim.
func (a *Allocator) Allocate(ctx context.Context, claim *resourcev1.ResourceClaim) (*resourcev1.AllocationResult, error) {
	if claim == nil {
		return nil, nil
	}

	classes, err := loadDeviceClasses(ctx, a.client, claim)
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

func loadDeviceClasses(ctx context.Context, cl client.Client, claim *resourcev1.ResourceClaim) (map[string]*resourcev1.DeviceClass, error) {
	names := deviceClassNames(claim)
	if len(names) == 0 {
		return nil, nil
	}

	classes := make(map[string]*resourcev1.DeviceClass, len(names))
	for _, name := range names {
		obj := &resourcev1.DeviceClass{}
		if err := cl.Get(ctx, client.ObjectKey{Name: name}, obj); err != nil {
			return nil, fmt.Errorf("deviceclass %q: %w", name, err)
		}
		classes[name] = obj
	}
	return classes, nil
}

func deviceClassNames(claim *resourcev1.ResourceClaim) []string {
	if claim == nil {
		return nil
	}
	seen := map[string]struct{}{}
	for _, req := range claim.Spec.Devices.Requests {
		if req.Exactly != nil && req.Exactly.DeviceClassName != "" {
			seen[req.Exactly.DeviceClassName] = struct{}{}
		}
		for _, sub := range req.FirstAvailable {
			if sub.DeviceClassName != "" {
				seen[sub.DeviceClassName] = struct{}{}
			}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	return names
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
