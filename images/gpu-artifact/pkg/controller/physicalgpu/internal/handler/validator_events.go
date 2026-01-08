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

package handler

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

func (h *ValidatorHandler) recordDriverReadyEvent(obj *gpuv1alpha1.PhysicalGPU, prev *gpuv1alpha1.PhysicalGPU) {
	if h.recorder == nil || obj == nil {
		return
	}

	newCond := meta.FindStatusCondition(obj.Status.Conditions, conditionDriverReady.String())
	if newCond == nil {
		return
	}

	var oldCond *metav1.Condition
	if prev != nil {
		oldCond = meta.FindStatusCondition(prev.Status.Conditions, conditionDriverReady.String())
	}

	if oldCond != nil &&
		oldCond.Status == newCond.Status &&
		oldCond.Reason == newCond.Reason &&
		oldCond.Message == newCond.Message {
		return
	}

	eventType := corev1.EventTypeWarning
	if newCond.Status == metav1.ConditionTrue {
		eventType = corev1.EventTypeNormal
	}

	h.recorder.Event(obj, eventType, newCond.Reason, newCond.Message)
}
