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

// FinalPrepareStep ends the prepare pipeline.
type FinalPrepareStep struct{}

// NewFinalPrepareStep constructs the final prepare step.
func NewFinalPrepareStep() FinalPrepareStep {
	return FinalPrepareStep{}
}

func (FinalPrepareStep) Take(_ context.Context, st *state.PrepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("prepare state is nil")
	}
	if st.Result.ClaimUID == "" {
		return nil, errors.New("prepare result is empty")
	}
	res := reconcile.Result{}
	return &res, nil
}

// FinalUnprepareStep ends the unprepare pipeline.
type FinalUnprepareStep struct{}

// NewFinalUnprepareStep constructs the final unprepare step.
func NewFinalUnprepareStep() FinalUnprepareStep {
	return FinalUnprepareStep{}
}

func (FinalUnprepareStep) Take(_ context.Context, _ *state.UnprepareState) (*reconcile.Result, error) {
	res := reconcile.Result{}
	return &res, nil
}
