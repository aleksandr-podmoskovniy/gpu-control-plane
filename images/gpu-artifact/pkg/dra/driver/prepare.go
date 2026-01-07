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

package driver

import (
	"context"
	"errors"
	"fmt"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"

	k8sprepare "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/k8s/prepare"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

// PrepareResourceClaims prepares claims and returns CDI device ids.
func (d *Driver) PrepareResourceClaims(ctx context.Context, claims []*resourceapi.ResourceClaim) (map[types.UID]kubeletplugin.PrepareResult, error) {
	results := make(map[types.UID]kubeletplugin.PrepareResult, len(claims))
	if len(claims) == 0 {
		return results, nil
	}

	slices, err := d.listResourceSlices(ctx)
	if err != nil {
		for _, claim := range claims {
			results[claim.UID] = prepareErrorResult(err)
		}
		return results, nil
	}

	for _, claim := range claims {
		results[claim.UID] = d.prepareClaim(ctx, claim, slices)
	}
	return results, nil
}

// UnprepareResourceClaims removes CDI specs for claims.
func (d *Driver) UnprepareResourceClaims(ctx context.Context, claims []kubeletplugin.NamespacedObject) (map[types.UID]error, error) {
	results := make(map[types.UID]error, len(claims))
	for _, claim := range claims {
		if err := d.unprepareClaim(ctx, claim); err != nil {
			results[claim.UID] = err
			continue
		}
		results[claim.UID] = nil
	}
	return results, nil
}

func (d *Driver) prepareClaim(ctx context.Context, claim *resourceapi.ResourceClaim, slices []resourceapi.ResourceSlice) kubeletplugin.PrepareResult {
	if claim == nil {
		return prepareErrorResult(errors.New("claim is nil"))
	}
	req, err := k8sprepare.BuildPrepareRequest(claim, d.driverName, d.nodeName, slices)
	if err != nil {
		return prepareErrorResult(err)
	}
	vfioConfigRequested, err := vfioRequestedFromConfig(claim, d.driverName)
	if err != nil {
		return prepareErrorResult(err)
	}
	req.VFIO = VFIORequested(claim.Annotations) || vfioConfigRequested

	result, err := d.prepareService.Prepare(ctx, req)
	if err != nil {
		return prepareErrorResult(err)
	}

	devices := make([]kubeletplugin.Device, 0, len(result.Devices))
	for _, dev := range result.Devices {
		devices = append(devices, kubeletplugin.Device{
			Requests:     []string{dev.Request},
			PoolName:     dev.Pool,
			DeviceName:   dev.Device,
			CDIDeviceIDs: dev.CDIDeviceIDs,
		})
	}

	logger.FromContext(ctx).Info(
		"claim prepared",
		"claim", claim.Name,
		"namespace", claim.Namespace,
		"deviceCount", len(devices),
	)
	if d.deviceStatusEnabled {
		if err := d.updateClaimDeviceStatus(ctx, claim); err != nil {
			logger.FromContext(ctx).Warn(
				"failed to update claim device status",
				"claim", claim.Name,
				"namespace", claim.Namespace,
				logger.SlogErr(err),
			)
		}
	}

	return kubeletplugin.PrepareResult{Devices: devices}
}

func (d *Driver) unprepareClaim(ctx context.Context, claim kubeletplugin.NamespacedObject) error {
	if d == nil || d.prepareService == nil {
		return errors.New("prepare service is not configured")
	}
	logger.FromContext(ctx).Info("claim unprepare", "claim", claim.Name, "namespace", claim.Namespace)
	if err := d.prepareService.Unprepare(ctx, string(claim.UID)); err != nil {
		return err
	}
	if d.deviceStatusEnabled {
		if err := d.clearClaimDeviceStatus(ctx, claim); err != nil {
			logger.FromContext(ctx).Warn(
				"failed to clear claim device status",
				"claim", claim.Name,
				"namespace", claim.Namespace,
				logger.SlogErr(err),
			)
		}
	}
	return nil
}

func prepareErrorResult(err error) kubeletplugin.PrepareResult {
	if err == nil {
		return kubeletplugin.PrepareResult{}
	}
	return kubeletplugin.PrepareResult{
		Err: fmt.Errorf("prepare failed: %v: %w", err, kubeletplugin.ErrRecoverable),
	}
}
