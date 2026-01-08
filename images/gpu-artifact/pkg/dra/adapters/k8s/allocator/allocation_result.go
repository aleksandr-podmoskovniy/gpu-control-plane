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
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// AllocationOptions controls optional allocation fields gated by feature flags.
type AllocationOptions struct {
	IncludeBindingConditions   bool
	IncludeAllocationTimestamp bool
}

// BuildAllocationResult converts domain allocation into API allocation.
func BuildAllocationResult(claim *resourcev1.ResourceClaim, alloc *domain.AllocationResult, classes map[string]*resourcev1.DeviceClass) (*resourcev1.AllocationResult, error) {
	return BuildAllocationResultWithOptions(claim, alloc, classes, AllocationOptions{})
}

// BuildAllocationResultWithOptions converts domain allocation into API allocation with feature options.
func BuildAllocationResultWithOptions(claim *resourcev1.ResourceClaim, alloc *domain.AllocationResult, classes map[string]*resourcev1.DeviceClass, opts AllocationOptions) (*resourcev1.AllocationResult, error) {
	if claim == nil || alloc == nil {
		return nil, nil
	}

	results := make([]resourcev1.DeviceRequestAllocationResult, 0, len(alloc.Devices))
	for _, dev := range alloc.Devices {
		result := resourcev1.DeviceRequestAllocationResult{
			Request: dev.Request,
			Driver:  dev.Driver,
			Pool:    dev.Pool,
			Device:  dev.Device,
		}
		if opts.IncludeBindingConditions {
			if len(dev.BindingConditions) > 0 {
				result.BindingConditions = cloneStrings(dev.BindingConditions)
			}
			if len(dev.BindingFailureConditions) > 0 {
				result.BindingFailureConditions = cloneStrings(dev.BindingFailureConditions)
			}
		}
		if len(dev.ConsumedCapacity) > 0 {
			result.ConsumedCapacity = consumedCapacity(dev.ConsumedCapacity)
			if dev.ShareID != "" {
				shareID := types.UID(dev.ShareID)
				result.ShareID = &shareID
			} else {
				shareID := uuid.NewUUID()
				result.ShareID = &shareID
			}
		}
		results = append(results, result)
	}

	out := &resourcev1.AllocationResult{
		Devices: resourcev1.DeviceAllocationResult{
			Results: results,
		},
		NodeSelector: nodeSelectorForNode(alloc.NodeName),
	}
	if opts.IncludeAllocationTimestamp {
		ts := metav1.Now()
		out.AllocationTimestamp = &ts
	}

	config, err := buildAllocationConfig(claim, classes)
	if err != nil {
		return nil, err
	}
	out.Devices.Config = config
	return out, nil
}
