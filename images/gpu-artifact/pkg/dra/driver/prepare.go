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

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
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
