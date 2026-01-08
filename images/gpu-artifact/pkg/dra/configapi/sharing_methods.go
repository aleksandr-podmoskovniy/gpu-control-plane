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

package configapi

import "fmt"

// IsTimeSlicing checks if the TimeSlicing strategy is applied.
func (s *GpuSharing) IsTimeSlicing() bool {
	if s == nil {
		return false
	}
	return s.Strategy == TimeSlicingStrategy
}

// IsMps checks if the MPS strategy is applied.
func (s *GpuSharing) IsMps() bool {
	if s == nil {
		return false
	}
	return s.Strategy == MpsStrategy
}

// IsTimeSlicing checks if the TimeSlicing strategy is applied.
func (s *MigDeviceSharing) IsTimeSlicing() bool {
	if s == nil {
		return false
	}
	return s.Strategy == TimeSlicingStrategy
}

// IsMps checks if the MPS strategy is applied.
func (s *MigDeviceSharing) IsMps() bool {
	if s == nil {
		return false
	}
	return s.Strategy == MpsStrategy
}

// GetTimeSlicingConfig returns the timeslicing config that applies to the given strategy.
func (s *GpuSharing) GetTimeSlicingConfig() (*TimeSlicingConfig, error) {
	if s == nil {
		return nil, fmt.Errorf("no sharing set to get config from")
	}
	if s.Strategy != TimeSlicingStrategy {
		return nil, fmt.Errorf("strategy is not set to '%v'", TimeSlicingStrategy)
	}
	if s.MpsConfig != nil {
		return nil, fmt.Errorf("cannot use MpsConfig with the '%v' strategy", TimeSlicingStrategy)
	}
	return s.TimeSlicingConfig, nil
}

// GetTimeSlicingConfig returns the timeslicing config that applies to the given strategy.
func (s *MigDeviceSharing) GetTimeSlicingConfig() (*TimeSlicingConfig, error) {
	return nil, nil
}

// GetMpsConfig returns the MPS config that applies to the given strategy.
func (s *GpuSharing) GetMpsConfig() (*MpsConfig, error) {
	if s == nil {
		return nil, fmt.Errorf("no sharing set to get config from")
	}
	if s.Strategy != MpsStrategy {
		return nil, fmt.Errorf("strategy is not set to '%v'", MpsStrategy)
	}
	if s.TimeSlicingConfig != nil {
		return nil, fmt.Errorf("cannot use TimeSlicingConfig with the '%v' strategy", MpsStrategy)
	}
	return s.MpsConfig, nil
}

// GetMpsConfig returns the MPS config that applies to the given strategy.
func (s *MigDeviceSharing) GetMpsConfig() (*MpsConfig, error) {
	if s == nil {
		return nil, fmt.Errorf("no sharing set to get config from")
	}
	if s.Strategy != MpsStrategy {
		return nil, fmt.Errorf("strategy is not set to '%v'", MpsStrategy)
	}
	return s.MpsConfig, nil
}
