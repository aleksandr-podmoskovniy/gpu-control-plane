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

package state

import "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"

// PrepareState captures intermediate state for a prepare pipeline.
type PrepareState struct {
	Request          domain.PrepareRequest
	MutableRequest   domain.PrepareRequest
	Checkpoint       domain.PrepareCheckpoint
	Claim            domain.PreparedClaim
	DeviceMap        map[string]domain.PrepareDevice
	DeviceStates     []domain.PreparedDeviceState
	Result           domain.PrepareResult
	Unlock           func() error
	ResourcesChanged bool
}

// NewPrepareState initializes the prepare state.
func NewPrepareState(req domain.PrepareRequest) *PrepareState {
	return &PrepareState{
		Request:        req,
		MutableRequest: req,
		DeviceMap:      map[string]domain.PrepareDevice{},
	}
}
