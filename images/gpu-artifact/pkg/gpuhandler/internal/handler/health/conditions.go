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

package health

import (
	"k8s.io/apimachinery/pkg/api/meta"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler"
)

func hardwareConditionChanged(prev, next *gpuv1alpha1.PhysicalGPU) bool {
	if prev == nil || next == nil {
		return false
	}

	oldCond := meta.FindStatusCondition(prev.Status.Conditions, handler.HardwareHealthyType)
	newCond := meta.FindStatusCondition(next.Status.Conditions, handler.HardwareHealthyType)
	if newCond == nil {
		return false
	}
	if oldCond == nil {
		return true
	}
	return oldCond.Status != newCond.Status ||
		oldCond.Reason != newCond.Reason ||
		oldCond.Message != newCond.Message ||
		oldCond.ObservedGeneration != newCond.ObservedGeneration
}
