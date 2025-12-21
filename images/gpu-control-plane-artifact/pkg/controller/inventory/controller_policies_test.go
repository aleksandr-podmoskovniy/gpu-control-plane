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

package inventory

import (
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
	"github.com/go-logr/logr/testr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func TestNewAppliesDefaultsAndPolicies(t *testing.T) {
	module := defaultModuleSettings()
	cfg := config.ControllerConfig{Workers: 0, ResyncPeriod: 0}

	rec, err := New(testr.New(t), cfg, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	if rec.cfg.Workers != 1 {
		t.Fatalf("expected workers default to 1, got %d", rec.cfg.Workers)
	}
	if rec.resyncPeriod != defaultResyncPeriod {
		t.Fatalf("expected default resync period %s, got %s", defaultResyncPeriod, rec.resyncPeriod)
	}
	if rec.fallbackManaged.LabelKey != module.ManagedNodes.LabelKey {
		t.Fatalf("unexpected managed label key %s", rec.fallbackManaged.LabelKey)
	}
	if string(rec.fallbackApproval.Mode) != string(module.DeviceApproval.Mode) {
		t.Fatalf("unexpected approval mode %s", rec.fallbackApproval.Mode)
	}
}

func TestNewReturnsErrorOnInvalidSelector(t *testing.T) {
	state := moduleconfig.DefaultState()
	state.Settings.DeviceApproval.Mode = moduleconfig.DeviceApprovalModeSelector
	state.Settings.DeviceApproval.Selector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "gpu.deckhouse.io/device.vendor", Operator: metav1.LabelSelectorOperator("Invalid")},
		},
	}
	store := moduleconfig.NewModuleConfigStore(state)

	_, err := New(testr.New(t), config.ControllerConfig{}, store, nil)
	if err == nil {
		t.Fatalf("expected error due to invalid selector")
	}
}

func TestCurrentPoliciesUsesStoreState(t *testing.T) {
	store := moduleconfig.NewModuleConfigStore(moduleconfig.DefaultState())
	rec, err := New(testr.New(t), config.ControllerConfig{}, store, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}

	updated := moduleconfig.DefaultState()
	updated.Settings.ManagedNodes.LabelKey = "gpu.deckhouse.io/custom"
	updated.Settings.ManagedNodes.EnabledByDefault = false
	updated.Settings.DeviceApproval.Mode = moduleconfig.DeviceApprovalModeSelector
	updated.Settings.DeviceApproval.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"gpu.deckhouse.io/product": "a100"},
	}
	store.Update(updated)

	managed, approval := rec.currentPolicies()
	if managed.LabelKey != "gpu.deckhouse.io/custom" || managed.EnabledByDefault {
		t.Fatalf("expected managed settings from store, got %+v", managed)
	}
	if !approval.AutoAttach(true, labels.Set{"gpu.deckhouse.io/product": "a100"}) {
		t.Fatalf("expected selector-based auto attach to match labels")
	}
}

func TestCurrentPoliciesFallsBackOnError(t *testing.T) {
	store := moduleconfig.NewModuleConfigStore(moduleconfig.DefaultState())
	rec, err := New(testr.New(t), config.ControllerConfig{}, store, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}

	fallbackManaged := rec.fallbackManaged
	fallbackApproval := rec.fallbackApproval
	rec.log = testr.New(t)

	invalid := moduleconfig.DefaultState()
	invalid.Settings.DeviceApproval.Mode = moduleconfig.DeviceApprovalModeSelector
	invalid.Settings.DeviceApproval.Selector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "", Operator: metav1.LabelSelectorOpIn},
		},
	}
	invalid.Settings.ManagedNodes.LabelKey = "  "
	store.Update(invalid)

	managed, approval := rec.currentPolicies()
	if managed != fallbackManaged {
		t.Fatalf("expected fallback managed policy, got %+v", managed)
	}
	if approval != fallbackApproval {
		t.Fatalf("expected fallback approval policy, got %+v", approval)
	}
}

func TestCurrentPoliciesWithoutStoreUsesFallback(t *testing.T) {
	rec, err := New(testr.New(t), config.ControllerConfig{}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}

	managed, approval := rec.currentPolicies()
	if managed != rec.fallbackManaged {
		t.Fatalf("expected fallback managed settings, got %+v", managed)
	}
	if approval != rec.fallbackApproval {
		t.Fatalf("expected fallback approval policy, got %+v", approval)
	}
}

func TestNewDefaultsLabelKeyWhenEmpty(t *testing.T) {
	module := defaultModuleSettings()
	module.ManagedNodes.LabelKey = "   "

	rec, err := New(testr.New(t), config.ControllerConfig{}, moduleStoreFrom(module), nil)
	if err != nil {
		t.Fatalf("unexpected error constructing reconciler: %v", err)
	}
	if rec.fallbackManaged.LabelKey != defaultManagedNodeLabelKey {
		t.Fatalf("expected label key defaulted to %s, got %s", defaultManagedNodeLabelKey, rec.fallbackManaged.LabelKey)
	}
}
