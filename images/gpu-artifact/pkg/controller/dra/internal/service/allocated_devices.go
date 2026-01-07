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
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	allocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

func listAllocatedDevices(ctx context.Context, cl client.Client) (map[allocator.DeviceKey]allocator.AllocatedDeviceInfo, error) {
	list := &resourcev1.ResourceClaimList{}
	if err := cl.List(ctx, list); err != nil {
		return nil, err
	}

	allocated := map[allocator.DeviceKey]allocator.AllocatedDeviceInfo{}
	for i := range list.Items {
		claim := &list.Items[i]
		if claim.Status.Allocation == nil {
			continue
		}
		for _, res := range claim.Status.Allocation.Devices.Results {
			key := allocator.DeviceKey{Driver: res.Driver, Pool: res.Pool, Device: res.Device}
			if res.ConsumedCapacity == nil || len(res.ConsumedCapacity) == 0 {
				allocated[key] = allocator.AllocatedDeviceInfo{Exclusive: true}
				continue
			}
			info := allocated[key]
			if info.Exclusive {
				continue
			}
			info.ConsumedCapacity = mergeConsumedCapacity(info.ConsumedCapacity, res.ConsumedCapacity)
			allocated[key] = info
		}
	}
	return allocated, nil
}

func mergeConsumedCapacity(dst map[string]resource.Quantity, src map[resourcev1.QualifiedName]resource.Quantity) map[string]resource.Quantity {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = map[string]resource.Quantity{}
	}
	for name, qty := range src {
		key := string(name)
		if current, ok := dst[key]; ok {
			current.Add(qty)
			dst[key] = current
			continue
		}
		dst[key] = qty.DeepCopy()
	}
	return dst
}
