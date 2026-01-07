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
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
)

// Service prepares and unprepares devices using node-local side effects.
type Service struct {
	cdi         ports.CDIWriter
	mig         ports.MigManager
	vfio        ports.VfioManager
	locker      ports.PrepareLocker
	checkpoints ports.PrepareCheckpointStore
}

// Options configure the prepare service.
type Options struct {
	CDI         ports.CDIWriter
	MIG         ports.MigManager
	VFIO        ports.VfioManager
	Locker      ports.PrepareLocker
	Checkpoints ports.PrepareCheckpointStore
}

// NewService creates a prepare Service.
func NewService(opts Options) (*Service, error) {
	if opts.CDI == nil {
		return nil, errors.New("CDI writer is required")
	}
	if opts.Locker == nil {
		return nil, errors.New("prepare locker is required")
	}
	if opts.Checkpoints == nil {
		return nil, errors.New("checkpoint store is required")
	}
	return &Service{
		cdi:         opts.CDI,
		mig:         opts.MIG,
		vfio:        opts.VFIO,
		locker:      opts.Locker,
		checkpoints: opts.Checkpoints,
	}, nil
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

	unlock, err := s.locker.Lock(ctx)
	if err != nil {
		return domain.PrepareResult{}, fmt.Errorf("acquire prepare lock: %w", err)
	}
	defer func() {
		_ = unlock()
	}()

	checkpoint, err := s.loadCheckpoint(ctx)
	if err != nil {
		return domain.PrepareResult{}, err
	}

	if claim, ok := checkpoint.Claims[req.ClaimUID]; ok && claim.State == domain.PrepareStateCompleted {
		return preparedResultFromClaim(req.ClaimUID, claim)
	}

	preparedClaim := checkpoint.Claims[req.ClaimUID]
	knownDevices := map[string]domain.PreparedDeviceState{}
	for _, dev := range preparedClaim.Devices {
		knownDevices[dev.Device] = dev
	}

	preparedStates := make([]domain.PreparedDeviceState, 0, len(req.Devices))
	mutableReq := req
	mutableReq.Devices = make([]domain.PrepareDevice, 0, len(req.Devices))

	for _, dev := range req.Devices {
		state, ok := knownDevices[dev.Device]
		if !ok {
			state = domain.PreparedDeviceState{
				Request: dev.Request,
				Pool:    dev.Pool,
				Device:  dev.Device,
			}
		}
		state.Request = dev.Request
		state.Pool = dev.Pool

		deviceType := attrString(dev.Attributes, allocatable.AttrDeviceType)
		if req.VFIO {
			if !isPhysicalDevice(deviceType) {
				return domain.PrepareResult{}, fmt.Errorf("vfio requested for non-physical device %q", dev.Device)
			}
			if dev.ShareID != "" || len(dev.ConsumedCapacity) > 0 {
				return domain.PrepareResult{}, fmt.Errorf("vfio requires exclusive allocation for device %q", dev.Device)
			}
			if s.vfio == nil {
				return domain.PrepareResult{}, errors.New("vfio manager is not configured")
			}
			if state.VFIO == nil {
				pci := attrString(dev.Attributes, allocatable.AttrPCIAddress)
				if pci == "" {
					return domain.PrepareResult{}, fmt.Errorf("pci address is missing for device %q", dev.Device)
				}
				vfioState, prepErr := s.vfio.Prepare(ctx, domain.VfioPrepareRequest{PCIBusID: pci})
				if prepErr != nil {
					return domain.PrepareResult{}, fmt.Errorf("vfio prepare %q: %w", dev.Device, prepErr)
				}
				state.VFIO = &vfioState
			}
		}

		if isMigDevice(deviceType) {
			if s.mig == nil {
				return domain.PrepareResult{}, errors.New("mig manager is not configured")
			}
			if state.MIG == nil {
				migReq, prepErr := buildMigPrepareRequest(dev)
				if prepErr != nil {
					return domain.PrepareResult{}, prepErr
				}
				migState, prepErr := s.mig.Prepare(ctx, migReq)
				if prepErr != nil {
					return domain.PrepareResult{}, fmt.Errorf("mig prepare %q: %w", dev.Device, prepErr)
				}
				state.MIG = &migState
			}
		}

		preparedStates = append(preparedStates, state)

		mutableDev := dev
		mutableDev.Attributes = cloneAttributes(dev.Attributes)
		if state.MIG != nil {
			mutableDev.Attributes[allocatable.AttrMigUUID] = allocatable.AttributeValue{String: &state.MIG.DeviceUUID}
		}
		mutableReq.Devices = append(mutableReq.Devices, mutableDev)
	}

	checkpoint.Claims[req.ClaimUID] = domain.PreparedClaim{
		State:   domain.PrepareStateStarted,
		Devices: preparedStates,
	}
	if err := s.checkpoints.Save(ctx, checkpoint); err != nil {
		return domain.PrepareResult{}, fmt.Errorf("save prepare checkpoint: %w", err)
	}

	deviceIDs, err := s.cdi.Write(ctx, mutableReq)
	if err != nil {
		return domain.PrepareResult{}, err
	}

	resultDevices := make([]domain.PreparedDevice, 0, len(preparedStates))
	for i, state := range preparedStates {
		ids, ok := deviceIDs[state.Device]
		if !ok {
			return domain.PrepareResult{}, fmt.Errorf("missing CDI ids for device %q", state.Device)
		}
		preparedStates[i].CDIDeviceIDs = ids
		resultDevices = append(resultDevices, domain.PreparedDevice{
			Request:      state.Request,
			Pool:         state.Pool,
			Device:       state.Device,
			CDIDeviceIDs: ids,
		})
	}

	checkpoint.Claims[req.ClaimUID] = domain.PreparedClaim{
		State:   domain.PrepareStateCompleted,
		Devices: preparedStates,
	}
	if err := s.checkpoints.Save(ctx, checkpoint); err != nil {
		return domain.PrepareResult{}, fmt.Errorf("save completed checkpoint: %w", err)
	}

	return domain.PrepareResult{
		ClaimUID: req.ClaimUID,
		Devices:  resultDevices,
	}, nil
}

// Unprepare removes CDI specs for the claim.
func (s *Service) Unprepare(ctx context.Context, claimUID string) error {
	if s == nil || s.cdi == nil {
		return errors.New("prepare service is not configured")
	}
	if claimUID == "" {
		return errors.New("claim UID is required")
	}

	unlock, err := s.locker.Lock(ctx)
	if err != nil {
		return fmt.Errorf("acquire prepare lock: %w", err)
	}
	defer func() {
		_ = unlock()
	}()

	checkpoint, err := s.loadCheckpoint(ctx)
	if err != nil {
		return err
	}

	claim, ok := checkpoint.Claims[claimUID]
	if !ok {
		return nil
	}
	if claim.State == domain.PrepareStateStarted {
		return nil
	}

	for _, dev := range claim.Devices {
		if dev.MIG != nil && s.mig != nil {
			if err := s.mig.Unprepare(ctx, *dev.MIG); err != nil {
				return fmt.Errorf("mig unprepare %q: %w", dev.Device, err)
			}
		}
		if dev.VFIO != nil && s.vfio != nil {
			if err := s.vfio.Unprepare(ctx, *dev.VFIO); err != nil {
				return fmt.Errorf("vfio unprepare %q: %w", dev.Device, err)
			}
		}
	}

	if err := s.cdi.Delete(ctx, claimUID); err != nil {
		return err
	}

	delete(checkpoint.Claims, claimUID)
	if err := s.checkpoints.Save(ctx, checkpoint); err != nil {
		return fmt.Errorf("save checkpoint cleanup: %w", err)
	}

	return nil
}
