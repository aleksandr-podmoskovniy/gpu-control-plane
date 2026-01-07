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

package prepare

import (
	"errors"
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// BuildPrepareRequest converts a claim and resource slices into a prepare request.
func BuildPrepareRequest(claim *resourcev1.ResourceClaim, driverName, nodeName string, slices []resourcev1.ResourceSlice) (domain.PrepareRequest, error) {
	if claim == nil {
		return domain.PrepareRequest{}, errors.New("claim is nil")
	}
	if claim.UID == "" {
		return domain.PrepareRequest{}, errors.New("claim UID is empty")
	}
	if claim.Status.Allocation == nil {
		return domain.PrepareRequest{}, fmt.Errorf("claim %q has no allocation", claim.Name)
	}
	results := claim.Status.Allocation.Devices.Results
	if len(results) == 0 {
		return domain.PrepareRequest{}, fmt.Errorf("claim %q has no allocated devices", claim.Name)
	}

	filtered := filterResults(results, driverName)
	if len(filtered) == 0 {
		return domain.PrepareRequest{}, fmt.Errorf("claim %q has no devices for driver %q", claim.Name, driverName)
	}

	deviceIndex := indexDevices(slices, driverName, nodeName)
	devices := make([]domain.PrepareDevice, 0, len(filtered))
	for _, res := range filtered {
		pool := deviceIndex[res.Pool]
		if pool == nil {
			return domain.PrepareRequest{}, fmt.Errorf("pool %q not found for device %q", res.Pool, res.Device)
		}
		dev, ok := pool.devices[res.Device]
		if !ok {
			return domain.PrepareRequest{}, fmt.Errorf("device %q not found in pool %q", res.Device, res.Pool)
		}
		devices = append(devices, domain.PrepareDevice{
			Request:          res.Request,
			Driver:           res.Driver,
			Pool:             res.Pool,
			Device:           res.Device,
			ShareID:          shareID(res.ShareID),
			ConsumedCapacity: consumedCapacity(res.ConsumedCapacity),
			Attributes:       attributesFromDevice(dev),
		})
	}

	return domain.PrepareRequest{
		ClaimUID: string(claim.UID),
		NodeName: nodeName,
		Devices:  devices,
	}, nil
}
