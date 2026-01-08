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

// UnprepareState captures intermediate state for an unprepare pipeline.
type UnprepareState struct {
	ClaimUID         string
	Checkpoint       domain.PrepareCheckpoint
	Claim            domain.PreparedClaim
	Unlock           func() error
	Skip             bool
	ResourcesChanged bool
}

// NewUnprepareState initializes the unprepare state.
func NewUnprepareState(claimUID string) *UnprepareState {
	return &UnprepareState{ClaimUID: claimUID}
}
