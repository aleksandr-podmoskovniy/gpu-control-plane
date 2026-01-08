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

package step

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// ValidatePrepareStep validates the prepare request.
type ValidatePrepareStep struct{}

// NewValidatePrepareStep constructs a validation step for prepare.
func NewValidatePrepareStep() ValidatePrepareStep {
	return ValidatePrepareStep{}
}

func (ValidatePrepareStep) Take(_ context.Context, st *state.PrepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("prepare state is nil")
	}
	if st.Request.ClaimUID == "" {
		return nil, errors.New("claim UID is required")
	}
	if len(st.Request.Devices) == 0 {
		return nil, errors.New("no devices to prepare")
	}
	return nil, nil
}
