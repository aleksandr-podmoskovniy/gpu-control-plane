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

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
)

// DeleteCDIStep removes CDI specs for a claim.
type DeleteCDIStep struct {
	cdi ports.CDIWriter
}

// NewDeleteCDIStep constructs a CDI delete step.
func NewDeleteCDIStep(cdi ports.CDIWriter) DeleteCDIStep {
	return DeleteCDIStep{cdi: cdi}
}

func (s DeleteCDIStep) Take(ctx context.Context, st *state.UnprepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("unprepare state is nil")
	}
	if st.Skip {
		return nil, nil
	}
	if s.cdi == nil {
		return nil, errors.New("cdi writer is not configured")
	}
	if err := s.cdi.Delete(ctx, st.ClaimUID); err != nil {
		return nil, err
	}
	return nil, nil
}
