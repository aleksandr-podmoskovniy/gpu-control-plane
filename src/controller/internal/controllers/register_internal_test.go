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
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	ctrl "sigs.k8s.io/controller-runtime"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	moduleconfigpkg "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
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

	deps.InventoryHandlers.Register(testInventoryHandler{})
	if len(deps.InventoryHandlers.List()) != 1 {
		t.Fatal("registry must contain registered inventory handler")
	}
}

func TestEnsureRegistriesSetsDefaultClusterController(t *testing.T) {
	orig := newClusterPoolController
	t.Cleanup(func() { newClusterPoolController = orig })

	newClusterPoolController = nil
	deps := Dependencies{Logger: testr.New(t)}
	ensureRegistries(&deps)
	if newClusterPoolController == nil {
		t.Fatal("newClusterPoolController must be initialised by ensureRegistries")
	}
	store := config.NewModuleConfigStore(moduleconfigpkg.DefaultState())
	if _, err := newClusterPoolController(testr.New(t), config.ControllerConfig{}, store, nil); err != nil {
		t.Fatalf("default cluster pool controller should be constructible: %v", err)
	}
}

func TestDefaultControllerConstructors(t *testing.T) {
	store := config.NewModuleConfigStore(moduleconfigpkg.DefaultState())

	if _, err := newModuleConfigController(testr.New(t), store); err != nil {
		t.Fatalf("newModuleConfigController returned error: %v", err)
	}
	if _, err := newInventoryController(testr.New(t), config.ControllerConfig{}, store, nil); err != nil {
		t.Fatalf("newInventoryController returned error: %v", err)
	}
	if _, err := newBootstrapController(testr.New(t), config.ControllerConfig{}, store, nil); err != nil {
		t.Fatalf("newBootstrapController returned error: %v", err)
	}
	if _, err := newPoolController(testr.New(t), config.ControllerConfig{}, store, nil); err != nil {
		t.Fatalf("newPoolController returned error: %v", err)
	}
}

type testInventoryHandler struct{}

func (testInventoryHandler) Name() string { return "test" }

func (testInventoryHandler) HandleDevice(context.Context, *v1alpha1.GPUDevice) (contracts.Result, error) {
	return contracts.Result{}, nil
}

type stubSetupController struct {
	err    error
	called int
}

func (s *stubSetupController) SetupWithManager(context.Context, ctrl.Manager) error {
	s.called++
	return s.err
}

func TestRegisterInvokesAllControllers(t *testing.T) {
	origModule := newModuleConfigController
	origInv := newInventoryController
	origBoot := newBootstrapController
	origPool := newPoolController
	origClusterPool := newClusterPoolController
	t.Cleanup(func() {
		newModuleConfigController = origModule
		newInventoryController = origInv
		newBootstrapController = origBoot
		newPoolController = origPool
		newClusterPoolController = origClusterPool
	})

	moduleStub := &stubSetupController{}
	invStub := &stubSetupController{}
	bootStub := &stubSetupController{}
	poolStub := &stubSetupController{}
	clusterPoolStub := &stubSetupController{}

	newModuleConfigController = func(logr.Logger, *config.ModuleConfigStore) (setupController, error) {
		return moduleStub, nil
	}
	newInventoryController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.InventoryHandler) (setupController, error) {
		return invStub, nil
	}
	newBootstrapController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.BootstrapHandler) (setupController, error) {
		return bootStub, nil
	}
	newPoolController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.PoolHandler) (setupController, error) {
		return poolStub, nil
	}
	newClusterPoolController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.PoolHandler) (setupController, error) {
		return clusterPoolStub, nil
	}

	store := config.NewModuleConfigStore(moduleconfigpkg.DefaultState())
	deps := Dependencies{Logger: testr.New(t)}
	if err := Register(context.Background(), nil, config.ControllersConfig{}, store, deps); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	expectedCalls := map[string]*stubSetupController{
		"moduleconfig": moduleStub,
		"inventory":    invStub,
		"bootstrap":    bootStub,
		"gpupool":      poolStub,
		"clusterpool":  clusterPoolStub,
	}
	for name, stub := range expectedCalls {
		if stub.called != 1 {
			t.Fatalf("%s controller SetupWithManager not invoked", name)
		}
	}
}

func TestRegisterPropagatesConstructorErrors(t *testing.T) {
	origModule := newModuleConfigController
	origInv := newInventoryController
	origBoot := newBootstrapController
	origPool := newPoolController
	origClusterPool := newClusterPoolController
	t.Cleanup(func() {
		newModuleConfigController = origModule
		newInventoryController = origInv
		newBootstrapController = origBoot
		newPoolController = origPool
		newClusterPoolController = origClusterPool
	})

	store := config.NewModuleConfigStore(moduleconfigpkg.DefaultState())

	testCases := []struct {
		name      string
		configure func(*testing.T)
	}{
		{
			name: "module config constructor error",
			configure: func(t *testing.T) {
				newModuleConfigController = func(logr.Logger, *config.ModuleConfigStore) (setupController, error) {
					return nil, errors.New("module config constructor")
				}
				newInventoryController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.InventoryHandler) (setupController, error) {
					t.Fatal("inventory constructor should not run after module config failure")
					return nil, nil
				}
			},
		},
		{
			name: "inventory constructor error",
			configure: func(t *testing.T) {
				newModuleConfigController = func(logr.Logger, *config.ModuleConfigStore) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newInventoryController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.InventoryHandler) (setupController, error) {
					return nil, errors.New("inventory constructor")
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.BootstrapHandler) (setupController, error) {
					t.Fatal("bootstrap should not run after inventory failure")
					return nil, nil
				}
			},
		},
		{
			name: "bootstrap constructor error",
			configure: func(t *testing.T) {
				newModuleConfigController = func(logr.Logger, *config.ModuleConfigStore) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newInventoryController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.InventoryHandler) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.BootstrapHandler) (setupController, error) {
					return nil, errors.New("bootstrap constructor")
				}
				newPoolController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.PoolHandler) (setupController, error) {
					t.Fatal("pool should not run after bootstrap failure")
					return nil, nil
				}
			},
		},
		{
			name: "pool constructor error",
			configure: func(t *testing.T) {
				newModuleConfigController = func(logr.Logger, *config.ModuleConfigStore) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newInventoryController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.InventoryHandler) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.BootstrapHandler) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newPoolController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.PoolHandler) (setupController, error) {
					return nil, errors.New("pool constructor")
				}
				newClusterPoolController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.PoolHandler) (setupController, error) {
					t.Fatal("cluster pool should not run after pool failure")
					return nil, nil
				}
			},
		},
		{
			name: "cluster pool constructor error",
			configure: func(t *testing.T) {
				newModuleConfigController = func(logr.Logger, *config.ModuleConfigStore) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newInventoryController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.InventoryHandler) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.BootstrapHandler) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newPoolController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.PoolHandler) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newClusterPoolController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.PoolHandler) (setupController, error) {
					return nil, errors.New("cluster pool constructor")
				}
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			newModuleConfigController = origModule
			newInventoryController = origInv
			newBootstrapController = origBoot
			newPoolController = origPool

			tc.configure(t)
			err := Register(context.Background(), nil, config.ControllersConfig{}, store, Dependencies{Logger: testr.New(t)})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestRegisterPropagatesSetupErrors(t *testing.T) {
	origModule := newModuleConfigController
	origInv := newInventoryController
	origBoot := newBootstrapController
	origPool := newPoolController
	t.Cleanup(func() {
		newModuleConfigController = origModule
		newInventoryController = origInv
		newBootstrapController = origBoot
		newPoolController = origPool
	})

	store := config.NewModuleConfigStore(moduleconfigpkg.DefaultState())

	buildStub := func(err error) setupController {
		return &stubSetupController{err: err}
	}

	testCases := []struct {
		name          string
		configure     func(*testing.T)
		expectedError string
	}{
		{
			name: "module config setup error",
			configure: func(t *testing.T) {
				newModuleConfigController = func(logr.Logger, *config.ModuleConfigStore) (setupController, error) {
					return buildStub(errors.New("module config setup")), nil
				}
				newInventoryController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.InventoryHandler) (setupController, error) {
					t.Fatal("inventory should not be called after module config failure")
					return nil, nil
				}
			},
			expectedError: "module config setup",
		},
		{
			name: "inventory setup error",
			configure: func(t *testing.T) {
				newModuleConfigController = func(logr.Logger, *config.ModuleConfigStore) (setupController, error) {
					return buildStub(nil), nil
				}
				newInventoryController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.InventoryHandler) (setupController, error) {
					return buildStub(errors.New("inventory setup")), nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.BootstrapHandler) (setupController, error) {
					t.Fatal("bootstrap should not be called after inventory failure")
					return nil, nil
				}
			},
			expectedError: "inventory setup",
		},
		{
			name: "bootstrap setup error",
			configure: func(t *testing.T) {
				newModuleConfigController = func(logr.Logger, *config.ModuleConfigStore) (setupController, error) {
					return buildStub(nil), nil
				}
				newInventoryController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.InventoryHandler) (setupController, error) {
					return buildStub(nil), nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.BootstrapHandler) (setupController, error) {
					return buildStub(errors.New("bootstrap setup")), nil
				}
				newPoolController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.PoolHandler) (setupController, error) {
					t.Fatal("pool should not be called after bootstrap failure")
					return nil, nil
				}
			},
			expectedError: "bootstrap setup",
		},
		{
			name: "pool setup error",
			configure: func(t *testing.T) {
				newModuleConfigController = func(logr.Logger, *config.ModuleConfigStore) (setupController, error) {
					return buildStub(nil), nil
				}
				newInventoryController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.InventoryHandler) (setupController, error) {
					return buildStub(nil), nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.BootstrapHandler) (setupController, error) {
					return buildStub(nil), nil
				}
				newPoolController = func(logr.Logger, config.ControllerConfig, *config.ModuleConfigStore, []contracts.PoolHandler) (setupController, error) {
					return buildStub(errors.New("pool setup")), nil
				}
			},
			expectedError: "pool setup",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			newModuleConfigController = origModule
			newInventoryController = origInv
			newBootstrapController = origBoot
			newPoolController = origPool

			tc.configure(t)

			err := Register(context.Background(), nil, config.ControllersConfig{}, store, Dependencies{Logger: testr.New(t)})
			if err == nil || err.Error() != tc.expectedError {
				t.Fatalf("expected error %q, got %v", tc.expectedError, err)
			}
		})
	}
}
