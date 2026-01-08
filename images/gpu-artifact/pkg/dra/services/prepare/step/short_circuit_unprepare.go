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

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// ShortCircuitUnprepareStep skips unprepare when there is nothing to do.
type ShortCircuitUnprepareStep struct{}

// NewShortCircuitUnprepareStep constructs a short-circuit step for unprepare.
func NewShortCircuitUnprepareStep() ShortCircuitUnprepareStep {
	return ShortCircuitUnprepareStep{}
}

func (ShortCircuitUnprepareStep) Take(_ context.Context, st *state.UnprepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, nil
	}
	if len(st.Claim.Devices) == 0 {
		st.Skip = true
		res := reconcile.Result{}
		return &res, nil
	}
	if st.Claim.State == domain.PrepareStateStarted {
		st.Skip = true
		res := reconcile.Result{}
		return &res, nil
	}
	return nil, nil
}
