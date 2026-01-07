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
	"context"
	"fmt"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

const checkpointVersion = "v1"

func (s *Service) loadCheckpoint(ctx context.Context) (domain.PrepareCheckpoint, error) {
	checkpoint, err := s.checkpoints.Load(ctx)
	if err != nil {
		return domain.PrepareCheckpoint{}, fmt.Errorf("load prepare checkpoint: %w", err)
	}
	if checkpoint.Version == "" {
		checkpoint.Version = checkpointVersion
	}
	if checkpoint.Claims == nil {
		checkpoint.Claims = map[string]domain.PreparedClaim{}
	}
	return checkpoint, nil
}

func preparedResultFromClaim(claimUID string, claim domain.PreparedClaim) (domain.PrepareResult, error) {
	if claim.State != domain.PrepareStateCompleted {
		return domain.PrepareResult{}, fmt.Errorf("claim %q is not prepared", claimUID)
	}
	devices := make([]domain.PreparedDevice, 0, len(claim.Devices))
	for _, dev := range claim.Devices {
		if len(dev.CDIDeviceIDs) == 0 {
			return domain.PrepareResult{}, fmt.Errorf("missing CDI ids for device %q", dev.Device)
		}
		devices = append(devices, domain.PreparedDevice{
			Request:      dev.Request,
			Pool:         dev.Pool,
			Device:       dev.Device,
			CDIDeviceIDs: dev.CDIDeviceIDs,
		})
	}
	return domain.PrepareResult{ClaimUID: claimUID, Devices: devices}, nil
}
