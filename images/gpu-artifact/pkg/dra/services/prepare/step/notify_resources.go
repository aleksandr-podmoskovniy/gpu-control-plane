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

// NotifyResourcesPrepareStep triggers ResourceSlice republish after prepare.
type NotifyResourcesPrepareStep struct {
	notifier ports.ResourcesChangeNotifier
}

// NewNotifyResourcesPrepareStep constructs a prepare notify step.
func NewNotifyResourcesPrepareStep(notifier ports.ResourcesChangeNotifier) NotifyResourcesPrepareStep {
	return NotifyResourcesPrepareStep{notifier: notifier}
}

func (s NotifyResourcesPrepareStep) Take(_ context.Context, st *state.PrepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("prepare state is nil")
	}
	if !st.ResourcesChanged || s.notifier == nil {
		return nil, nil
	}
	s.notifier.Notify()
	return nil, nil
}

// NotifyResourcesUnprepareStep triggers ResourceSlice republish after unprepare.
type NotifyResourcesUnprepareStep struct {
	notifier ports.ResourcesChangeNotifier
}

// NewNotifyResourcesUnprepareStep constructs an unprepare notify step.
func NewNotifyResourcesUnprepareStep(notifier ports.ResourcesChangeNotifier) NotifyResourcesUnprepareStep {
	return NotifyResourcesUnprepareStep{notifier: notifier}
}

func (s NotifyResourcesUnprepareStep) Take(_ context.Context, st *state.UnprepareState) (*reconcile.Result, error) {
	if st == nil {
		return nil, errors.New("unprepare state is nil")
	}
	if !st.ResourcesChanged || s.notifier == nil {
		return nil, nil
	}
	s.notifier.Notify()
	return nil, nil
}
