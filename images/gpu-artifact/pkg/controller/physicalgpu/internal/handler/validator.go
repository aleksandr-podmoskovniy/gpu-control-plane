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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/conditions"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/physicalgpu/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/controller/physicalgpu/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
)

const validatorHandlerName = "validator"
const labelVendor = "gpu.deckhouse.io/vendor"

// ValidatorHandler updates PhysicalGPU conditions based on validator readiness.
type ValidatorHandler struct {
	validator *service.Validator
	recorder  eventrecord.EventRecorderLogger
}

// NewValidatorHandler constructs a validator handler.
func NewValidatorHandler(validator *service.Validator, recorder eventrecord.EventRecorderLogger) *ValidatorHandler {
	return &ValidatorHandler{validator: validator, recorder: recorder}
}

// Name returns the handler name.
func (h *ValidatorHandler) Name() string {
	return validatorHandlerName
}

// Handle reconciles validator readiness for the PhysicalGPU.
func (h *ValidatorHandler) Handle(ctx context.Context, st state.PhysicalGPUState) (reconcile.Result, error) {
	if h.validator == nil || st.Resource.IsEmpty() {
		return reconcile.Result{}, nil
	}

	obj := st.Resource.Changed()
	nodeName := ""
	if obj.Status.NodeInfo != nil {
		nodeName = obj.Status.NodeInfo.NodeName
	}
	if nodeName == "" {
		return reconcile.Result{}, nil
	}

	if obj.Labels[labelVendor] != "nvidia" {
		conditions.RemoveCondition(conditionDriverReady, &obj.Status.Conditions)
		return reconcile.Result{}, nil
	}

	res, err := h.validator.Status(ctx, nodeName)
	if err != nil {
		return reconcile.Result{}, err
	}

	gen := obj.Generation
	driverReady := conditions.NewConditionBuilder(conditionDriverReady).Generation(gen)
	switch {
	case !res.Present:
		driverReady = driverReady.Status(metav1.ConditionUnknown).
			Reason(reasonValidatorMissing).
			Message(res.Message)
	case res.Ready:
		driverReady = driverReady.Status(metav1.ConditionTrue).
			Reason(reasonValidatorReady).
			Message("validator is ready")
	default:
		driverReady = driverReady.Status(metav1.ConditionFalse).
			Reason(reasonValidatorNotReady).
			Message(res.Message)
	}

	mgr := conditions.NewManager(obj.Status.Conditions)
	mgr.Update(driverReady.Condition())
	obj.Status.Conditions = mgr.Generate()

	h.recordDriverReadyEvent(obj, st.Resource.Current())
	return reconcile.Result{}, nil
}
