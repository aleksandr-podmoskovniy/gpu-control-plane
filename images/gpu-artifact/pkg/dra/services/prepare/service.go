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
	"errors"
	"fmt"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
)

// Service prepares and unprepares devices using node-local side effects.
type Service struct {
	cdi ports.CDIWriter
}

// NewService creates a prepare Service.
func NewService(cdi ports.CDIWriter) *Service {
	return &Service{cdi: cdi}
}

// Prepare writes CDI specs and returns prepared devices for the claim.
func (s *Service) Prepare(ctx context.Context, req domain.PrepareRequest) (domain.PrepareResult, error) {
	if s == nil || s.cdi == nil {
		return domain.PrepareResult{}, errors.New("CDI writer is not configured")
	}
	if req.ClaimUID == "" {
		return domain.PrepareResult{}, errors.New("claim UID is required")
	}
	if len(req.Devices) == 0 {
		return domain.PrepareResult{}, errors.New("no devices to prepare")
	}

	deviceIDs, err := s.cdi.Write(ctx, req)
	if err != nil {
		return domain.PrepareResult{}, err
	}

	prepared := make([]domain.PreparedDevice, 0, len(req.Devices))
	for _, dev := range req.Devices {
		ids, ok := deviceIDs[dev.Device]
		if !ok {
			return domain.PrepareResult{}, fmt.Errorf("missing CDI ids for device %q", dev.Device)
		}
		prepared = append(prepared, domain.PreparedDevice{
			Request:      dev.Request,
			Pool:         dev.Pool,
			Device:       dev.Device,
			CDIDeviceIDs: ids,
		})
	}

	return domain.PrepareResult{
		ClaimUID: req.ClaimUID,
		Devices:  prepared,
	}, nil
}

// Unprepare removes CDI specs for the claim.
func (s *Service) Unprepare(ctx context.Context, claimUID string) error {
	if s == nil || s.cdi == nil {
		return errors.New("CDI writer is not configured")
	}
	if claimUID == "" {
		return errors.New("claim UID is required")
	}
	return s.cdi.Delete(ctx, claimUID)
}
