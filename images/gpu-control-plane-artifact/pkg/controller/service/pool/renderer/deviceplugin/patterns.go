// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package deviceplugin

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
	poolsvc "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/deps"
)

// AssignedDevicePatterns returns sorted UUID patterns for devices assigned to the pool.
func AssignedDevicePatterns(ctx context.Context, d deps.Deps, pool *v1alpha1.GPUPool) []string {
	if d.Client == nil || pool == nil {
		return nil
	}

	allowedStates := map[v1alpha1.GPUDeviceState]struct{}{
		v1alpha1.GPUDeviceStatePendingAssignment: {},
		v1alpha1.GPUDeviceStateAssigned:          {},
		v1alpha1.GPUDeviceStateReserved:          {},
	}

	var devices v1alpha1.GPUDeviceList
	if err := d.Client.List(ctx, &devices, client.MatchingFields{indexer.GPUDevicePoolRefNameField: pool.Name}); err != nil {
		d.Log.Error(err, "list GPUDevices for pool patterns", "pool", pool.Name)
		return nil
	}

	patterns := make(map[string]struct{})
	for _, dev := range devices.Items {
		if poolsvc.IsDeviceIgnored(&dev) {
			continue
		}
		if !poolsvc.PoolRefMatchesPool(pool, dev.Status.PoolRef) {
			continue
		}
		if _, ok := allowedStates[dev.Status.State]; !ok {
			continue
		}
		uuid := trimUUID(dev.Status.Hardware.UUID)
		if uuid == "" {
			continue
		}
		patterns[uuid] = struct{}{}
	}

	return normalisePatterns(patterns)
}

// PoolHasAssignedDevices reports whether the pool has any managed devices.
func PoolHasAssignedDevices(ctx context.Context, d deps.Deps, pool *v1alpha1.GPUPool) (bool, error) {
	if d.Client == nil || pool == nil {
		return false, nil
	}

	var devices v1alpha1.GPUDeviceList
	if err := d.Client.List(ctx, &devices, client.MatchingFields{indexer.GPUDevicePoolRefNameField: pool.Name}); err != nil {
		return false, err
	}

	for i := range devices.Items {
		dev := &devices.Items[i]
		if poolsvc.IsDeviceIgnored(dev) {
			continue
		}
		if !poolsvc.PoolRefMatchesPool(pool, dev.Status.PoolRef) {
			continue
		}
		return true, nil
	}

	return false, nil
}
