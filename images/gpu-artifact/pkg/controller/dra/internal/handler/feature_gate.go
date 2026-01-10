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
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/dra/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/dra/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

const featureGateHandlerName = "feature-gate"
const reasonFeatureGateDisabled = "FeatureGateDisabled"

// FeatureGateHandler records DRA feature gate issues for DeviceClasses.
type FeatureGateHandler struct {
	classes  *service.DeviceClassService
	recorder eventrecord.EventRecorderLogger
	enabled  bool
}

// NewFeatureGateHandler constructs a feature gate handler.
func NewFeatureGateHandler(classes *service.DeviceClassService, recorder eventrecord.EventRecorderLogger, extendedResourceEnabled bool) *FeatureGateHandler {
	return &FeatureGateHandler{classes: classes, recorder: recorder, enabled: extendedResourceEnabled}
}

// Name returns the handler name.
func (h *FeatureGateHandler) Name() string {
	return featureGateHandlerName
}

// Handle records events when DeviceClass extendedResourceName is missing.
func (h *FeatureGateHandler) Handle(ctx context.Context, st *state.DRAState) (reconcile.Result, error) {
	if h.classes == nil || h.recorder == nil || st.Resource.IsEmpty() {
		return reconcile.Result{}, nil
	}

	if h.enabled {
		return reconcile.Result{}, nil
	}

	classes, err := h.classes.Load(ctx, st.Resource.Current())
	if err != nil {
		return reconcile.Result{}, err
	}

	for _, class := range classes {
		if class.Spec.ExtendedResourceName == nil || *class.Spec.ExtendedResourceName == "" {
			continue
		}
		msg := fmt.Sprintf("DeviceClass %q sets extendedResourceName while DRAExtendedResource is disabled", class.Name)
		log := logger.FromContext(ctx).With("deviceClass", class.Name)
		h.recorder.WithLogging(log).Event(class, corev1.EventTypeWarning, reasonFeatureGateDisabled, msg)
	}

	return reconcile.Result{}, nil
}
