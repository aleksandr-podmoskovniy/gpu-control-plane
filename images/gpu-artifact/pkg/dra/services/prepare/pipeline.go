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
	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/steptaker"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare/step"
)

func newPrepareSteps(opts Options) steptaker.StepTakers[*state.PrepareState] {
	return steptaker.NewStepTakers(
		step.NewValidatePrepareStep(),
		step.NewLockPrepareStep(opts.Locker),
		step.NewLoadPrepareCheckpointStep(opts.Checkpoints),
		step.NewShortCircuitPrepareStep(),
		step.NewPlanPrepareStep(opts.MIG, opts.VFIO),
		step.NewGPUFreeCheckStep(opts.GPUChecker),
		step.NewSavePrepareStartedStep(opts.Checkpoints),
		step.NewMigPrepareStep(opts.MIG),
		step.NewVfioPrepareStep(opts.VFIO),
		step.NewSharingPrepareStep(opts.TimeSlicing, opts.Mps),
		step.NewWriteCDIStep(opts.CDI),
		step.NewSavePrepareCompletedStep(opts.Checkpoints),
		step.NewNotifyResourcesPrepareStep(opts.ResourcesNotifier),
		step.NewFinalPrepareStep(),
	)
}

func newUnprepareSteps(opts Options) steptaker.StepTakers[*state.UnprepareState] {
	return steptaker.NewStepTakers(
		step.NewValidateUnprepareStep(),
		step.NewLockUnprepareStep(opts.Locker),
		step.NewLoadUnprepareCheckpointStep(opts.Checkpoints),
		step.NewShortCircuitUnprepareStep(),
		step.NewSharingUnprepareStep(opts.TimeSlicing, opts.Mps),
		step.NewMigUnprepareStep(opts.MIG),
		step.NewVfioUnprepareStep(opts.VFIO),
		step.NewDeleteCDIStep(opts.CDI),
		step.NewCleanupCheckpointStep(opts.Checkpoints),
		step.NewNotifyResourcesUnprepareStep(opts.ResourcesNotifier),
		step.NewFinalUnprepareStep(),
	)
}
