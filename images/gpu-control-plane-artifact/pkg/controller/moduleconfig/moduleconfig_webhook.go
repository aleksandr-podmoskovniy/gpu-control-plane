// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package moduleconfig

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mcapi "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig/api"
)

const moduleConfigName = "gpu-control-plane"

func SetupWebhookWithManager(mgr manager.Manager, log logr.Logger) error {
	if mgr.GetWebhookServer() == nil {
		return nil
	}
	validator := NewModuleConfigValidator(log)
	return builder.WebhookManagedBy(mgr).
		For(&mcapi.ModuleConfig{}).
		WithValidator(validator).
		Complete()
}

type ModuleConfigValidator struct {
	log logr.Logger
}

func NewModuleConfigValidator(log logr.Logger) *ModuleConfigValidator {
	return &ModuleConfigValidator{log: log.WithName("moduleconfig")}
}

func (v *ModuleConfigValidator) ValidateCreate(_ context.Context, obj runtime.Object) (cradmission.Warnings, error) {
	mc, ok := obj.(*mcapi.ModuleConfig)
	if !ok {
		return nil, fmt.Errorf("expected ModuleConfig but got %T", obj)
	}
	if mc.GetName() != moduleConfigName {
		return nil, nil
	}
	return nil, validateModuleConfig(mc)
}

func (v *ModuleConfigValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (cradmission.Warnings, error) {
	oldMC, ok := oldObj.(*mcapi.ModuleConfig)
	if !ok {
		return nil, fmt.Errorf("expected old ModuleConfig but got %T", oldObj)
	}
	newMC, ok := newObj.(*mcapi.ModuleConfig)
	if !ok {
		return nil, fmt.Errorf("expected new ModuleConfig but got %T", newObj)
	}
	if newMC.GetName() != moduleConfigName {
		return nil, nil
	}
	if oldMC.GetGeneration() == newMC.GetGeneration() {
		return nil, nil
	}
	return nil, validateModuleConfig(newMC)
}

func (v *ModuleConfigValidator) ValidateDelete(_ context.Context, obj runtime.Object) (cradmission.Warnings, error) {
	if _, ok := obj.(*mcapi.ModuleConfig); !ok {
		return nil, fmt.Errorf("expected ModuleConfig but got %T", obj)
	}
	return nil, nil
}

func validateModuleConfig(obj *mcapi.ModuleConfig) error {
	input := Input{
		Enabled:  obj.Spec.Enabled,
		Settings: map[string]any(obj.Spec.Settings),
	}
	if _, err := Parse(input); err != nil {
		return fmt.Errorf("parse module configuration: %w", err)
	}
	return nil
}

var _ cradmission.CustomValidator = (*ModuleConfigValidator)(nil)
