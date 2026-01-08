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
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

func setHardwareCondition(obj *gpuv1alpha1.PhysicalGPU, status metav1.ConditionStatus, reason, message string) {
	cond := metav1.Condition{
		Type:               handler.HardwareHealthyType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: obj.Generation,
	}
	meta.SetStatusCondition(&obj.Status.Conditions, cond)
}

func (h *CapabilitiesHandler) recordHardwareEvent(ctx context.Context, obj, base *gpuv1alpha1.PhysicalGPU, reason, message string) {
	if h.recorder == nil || obj == nil || base == nil {
		return
	}
	if !hardwareConditionChanged(base, obj) {
		return
	}

	log := logger.FromContext(ctx)
	if obj.Status.NodeInfo != nil && obj.Status.NodeInfo.NodeName != "" {
		log = log.With("node", obj.Status.NodeInfo.NodeName)
	}
	if obj.Status.PCIInfo != nil && obj.Status.PCIInfo.Address != "" {
		log = log.With("pci", obj.Status.PCIInfo.Address)
	}

	h.recorder.WithLogging(log).Event(
		obj,
		corev1.EventTypeWarning,
		reason,
		message,
	)
}
