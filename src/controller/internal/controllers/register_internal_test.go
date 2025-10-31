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

package controllers

import (
	"context"
	"testing"

	"github.com/go-logr/logr/testr"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

func TestEnsureRegistriesInitialisesDependencies(t *testing.T) {
	deps := Dependencies{Logger: testr.New(t)}
	ensureRegistries(&deps)

	if deps.InventoryHandlers == nil {
		t.Fatal("inventory registry must be initialised")
	}
	if deps.BootstrapHandlers == nil {
		t.Fatal("bootstrap registry must be initialised")
	}
	if deps.PoolHandlers == nil {
		t.Fatal("pool registry must be initialised")
	}
	if deps.AdmissionHandlers == nil {
		t.Fatal("admission registry must be initialised")
	}

	// verify registries work
	deps.InventoryHandlers.Register(testInventoryHandler{})
	if len(deps.InventoryHandlers.List()) != 1 {
		t.Fatal("registry List should contain registered handler")
	}
}

type testInventoryHandler struct{}

func (testInventoryHandler) Name() string { return "test" }

func (testInventoryHandler) HandleDevice(context.Context, *gpuv1alpha1.GPUDevice) (contracts.Result, error) {
	return contracts.Result{}, nil
}
