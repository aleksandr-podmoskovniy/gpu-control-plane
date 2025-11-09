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

package contracts

import (
	"context"
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

type stubNamed struct{ id string }

func (s stubNamed) Name() string { return s.id }

func TestRegistryRegisterAndList(t *testing.T) {
	r := NewRegistry[stubNamed]()
	first := stubNamed{id: "first"}
	second := stubNamed{id: "second"}

	r.Register(first)
	r.Register(second)

	items := r.List()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Register same name should replace
	r.Register(stubNamed{id: "first"})
	items = r.List()
	if len(items) != 2 {
		t.Fatalf("expected replacement to keep 2 items, got %d", len(items))
	}
}

type stubInventory struct{ stubNamed }

func (stubInventory) HandleDevice(context.Context, *v1alpha1.GPUDevice) (Result, error) {
	return Result{}, nil
}

type stubBootstrap struct{ stubNamed }

func (stubBootstrap) HandleNode(context.Context, *v1alpha1.GPUNodeInventory) (Result, error) {
	return Result{}, nil
}

type stubPool struct{ stubNamed }

func (stubPool) HandlePool(context.Context, *v1alpha1.GPUPool) (Result, error) {
	return Result{}, nil
}

type stubAdmission struct{ stubNamed }

func (stubAdmission) SyncPool(context.Context, *v1alpha1.GPUPool) (Result, error) {
	return Result{}, nil
}

func TestTypedRegistriesInitialise(t *testing.T) {
	inventoryReg := NewInventoryRegistry()
	if inventoryReg == nil || len(inventoryReg.List()) != 0 {
		t.Fatal("inventory registry must start empty")
	}

	bootstrapReg := NewBootstrapRegistry()
	if bootstrapReg == nil || len(bootstrapReg.List()) != 0 {
		t.Fatal("bootstrap registry must start empty")
	}

	poolReg := NewPoolRegistry()
	if poolReg == nil || len(poolReg.List()) != 0 {
		t.Fatal("pool registry must start empty")
	}

	admissionReg := NewAdmissionRegistry()
	if admissionReg == nil || len(admissionReg.List()) != 0 {
		t.Fatal("admission registry must start empty")
	}

	inventoryReg.Register(stubInventory{stubNamed{"inv"}})
	bootstrapReg.Register(stubBootstrap{stubNamed{"boot"}})
	poolReg.Register(stubPool{stubNamed{"pool"}})
	admissionReg.Register(stubAdmission{stubNamed{"adm"}})

	if len(inventoryReg.List()) != 1 || len(bootstrapReg.List()) != 1 || len(poolReg.List()) != 1 || len(admissionReg.List()) != 1 {
		t.Fatal("typed registries must register handlers")
	}
}
