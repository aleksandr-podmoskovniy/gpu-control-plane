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

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
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
func TestDefaultControllerConstructors(t *testing.T) {
	if _, err := newInventoryController(testr.New(t), config.ControllerConfig{}, config.ModuleSettings{}, nil); err != nil {
		t.Fatalf("newInventoryController returned error: %v", err)
	}
	if _, err := newBootstrapController(testr.New(t), config.ControllerConfig{}, nil); err != nil {
		t.Fatalf("newBootstrapController returned error: %v", err)
	}
	if _, err := newPoolController(testr.New(t), config.ControllerConfig{}, nil); err != nil {
		t.Fatalf("newPoolController returned error: %v", err)
	}
	if _, err := newAdmissionController(testr.New(t), config.ControllerConfig{}, nil); err != nil {
		t.Fatalf("newAdmissionController returned error: %v", err)
	}
}

type testInventoryHandler struct{}

func (testInventoryHandler) Name() string { return "test" }

func (testInventoryHandler) HandleDevice(context.Context, *gpuv1alpha1.GPUDevice) (contracts.Result, error) {
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
	origInv, origBoot, origPool, origAdm := newInventoryController, newBootstrapController, newPoolController, newAdmissionController
	t.Cleanup(func() {
		newInventoryController = origInv
		newBootstrapController = origBoot
		newPoolController = origPool
		newAdmissionController = origAdm
	})

	invStub := &stubSetupController{}
	bootStub := &stubSetupController{}
	poolStub := &stubSetupController{}
	admStub := &stubSetupController{}

	newInventoryController = func(logr.Logger, config.ControllerConfig, config.ModuleSettings, []contracts.InventoryHandler) (setupController, error) {
		return invStub, nil
	}
	newBootstrapController = func(logr.Logger, config.ControllerConfig, []contracts.BootstrapHandler) (setupController, error) {
		return bootStub, nil
	}
	newPoolController = func(logr.Logger, config.ControllerConfig, []contracts.PoolHandler) (setupController, error) {
		return poolStub, nil
	}
	newAdmissionController = func(logr.Logger, config.ControllerConfig, []contracts.AdmissionHandler) (setupController, error) {
		return admStub, nil
	}

	deps := Dependencies{Logger: testr.New(t)}
	if err := Register(context.Background(), nil, config.ControllersConfig{}, config.ModuleSettings{}, deps); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	for name, stub := range map[string]*stubSetupController{
		"inventory": invStub,
		"bootstrap": bootStub,
		"gpupool":   poolStub,
		"admission": admStub,
	} {
		if stub.called != 1 {
			t.Fatalf("%s controller SetupWithManager not invoked", name)
		}
	}
}

func TestRegisterPropagatesConstructorErrors(t *testing.T) {
	origInv, origBoot, origPool, origAdm := newInventoryController, newBootstrapController, newPoolController, newAdmissionController
	t.Cleanup(func() {
		newInventoryController = origInv
		newBootstrapController = origBoot
		newPoolController = origPool
		newAdmissionController = origAdm
	})

	testCases := []struct {
		name      string
		configure func(*testing.T)
	}{
		{
			name: "inventory constructor error",
			configure: func(t *testing.T) {
				newInventoryController = func(logr.Logger, config.ControllerConfig, config.ModuleSettings, []contracts.InventoryHandler) (setupController, error) {
					return nil, errors.New("inventory constructor")
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, []contracts.BootstrapHandler) (setupController, error) {
					t.Fatal("bootstrap constructor should not be called after inventory failure")
					return nil, nil
				}
			},
		},
		{
			name: "bootstrap constructor error",
			configure: func(t *testing.T) {
				newInventoryController = func(logr.Logger, config.ControllerConfig, config.ModuleSettings, []contracts.InventoryHandler) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, []contracts.BootstrapHandler) (setupController, error) {
					return nil, errors.New("bootstrap constructor")
				}
				newPoolController = func(logr.Logger, config.ControllerConfig, []contracts.PoolHandler) (setupController, error) {
					t.Fatal("pool constructor should not be called after bootstrap failure")
					return nil, nil
				}
			},
		},
		{
			name: "pool constructor error",
			configure: func(t *testing.T) {
				newInventoryController = func(logr.Logger, config.ControllerConfig, config.ModuleSettings, []contracts.InventoryHandler) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, []contracts.BootstrapHandler) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newPoolController = func(logr.Logger, config.ControllerConfig, []contracts.PoolHandler) (setupController, error) {
					return nil, errors.New("pool constructor")
				}
				newAdmissionController = func(logr.Logger, config.ControllerConfig, []contracts.AdmissionHandler) (setupController, error) {
					t.Fatal("admission constructor should not be called after pool failure")
					return nil, nil
				}
			},
		},
		{
			name: "admission constructor error",
			configure: func(t *testing.T) {
				newInventoryController = func(logr.Logger, config.ControllerConfig, config.ModuleSettings, []contracts.InventoryHandler) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, []contracts.BootstrapHandler) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newPoolController = func(logr.Logger, config.ControllerConfig, []contracts.PoolHandler) (setupController, error) {
					return &stubSetupController{}, nil
				}
				newAdmissionController = func(logr.Logger, config.ControllerConfig, []contracts.AdmissionHandler) (setupController, error) {
					return nil, errors.New("admission constructor")
				}
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			newInventoryController = origInv
			newBootstrapController = origBoot
			newPoolController = origPool
			newAdmissionController = origAdm

			tc.configure(t)
			err := Register(context.Background(), nil, config.ControllersConfig{}, config.ModuleSettings{}, Dependencies{Logger: testr.New(t)})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestRegisterPropagatesSetupErrors(t *testing.T) {
	origInv, origBoot, origPool, origAdm := newInventoryController, newBootstrapController, newPoolController, newAdmissionController
	t.Cleanup(func() {
		newInventoryController = origInv
		newBootstrapController = origBoot
		newPoolController = origPool
		newAdmissionController = origAdm
	})

	buildStub := func(err error) setupController {
		return &stubSetupController{err: err}
	}

	testCases := []struct {
		name          string
		configure     func()
		expectedError error
	}{
		{
			name: "inventory setup error",
			configure: func() {
				newInventoryController = func(logr.Logger, config.ControllerConfig, config.ModuleSettings, []contracts.InventoryHandler) (setupController, error) {
					return buildStub(errors.New("inventory setup")), nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, []contracts.BootstrapHandler) (setupController, error) {
					t.Fatal("bootstrap should not be called after inventory failure")
					return nil, nil
				}
			},
			expectedError: errors.New("inventory setup"),
		},
		{
			name: "bootstrap setup error",
			configure: func() {
				newInventoryController = func(logr.Logger, config.ControllerConfig, config.ModuleSettings, []contracts.InventoryHandler) (setupController, error) {
					return buildStub(nil), nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, []contracts.BootstrapHandler) (setupController, error) {
					return buildStub(errors.New("bootstrap setup")), nil
				}
				newPoolController = func(logr.Logger, config.ControllerConfig, []contracts.PoolHandler) (setupController, error) {
					t.Fatal("pool should not be called after bootstrap failure")
					return nil, nil
				}
			},
			expectedError: errors.New("bootstrap setup"),
		},
		{
			name: "pool setup error",
			configure: func() {
				newInventoryController = func(logr.Logger, config.ControllerConfig, config.ModuleSettings, []contracts.InventoryHandler) (setupController, error) {
					return buildStub(nil), nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, []contracts.BootstrapHandler) (setupController, error) {
					return buildStub(nil), nil
				}
				newPoolController = func(logr.Logger, config.ControllerConfig, []contracts.PoolHandler) (setupController, error) {
					return buildStub(errors.New("pool setup")), nil
				}
				newAdmissionController = func(logr.Logger, config.ControllerConfig, []contracts.AdmissionHandler) (setupController, error) {
					t.Fatal("admission should not be called after pool failure")
					return nil, nil
				}
			},
			expectedError: errors.New("pool setup"),
		},
		{
			name: "admission setup error",
			configure: func() {
				newInventoryController = func(logr.Logger, config.ControllerConfig, config.ModuleSettings, []contracts.InventoryHandler) (setupController, error) {
					return buildStub(nil), nil
				}
				newBootstrapController = func(logr.Logger, config.ControllerConfig, []contracts.BootstrapHandler) (setupController, error) {
					return buildStub(nil), nil
				}
				newPoolController = func(logr.Logger, config.ControllerConfig, []contracts.PoolHandler) (setupController, error) {
					return buildStub(nil), nil
				}
				newAdmissionController = func(logr.Logger, config.ControllerConfig, []contracts.AdmissionHandler) (setupController, error) {
					return buildStub(errors.New("admission setup")), nil
				}
			},
			expectedError: errors.New("admission setup"),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			newInventoryController = origInv
			newBootstrapController = origBoot
			newPoolController = origPool
			newAdmissionController = origAdm
			tc.configure()
			err := Register(context.Background(), nil, config.ControllersConfig{}, config.ModuleSettings{}, Dependencies{Logger: testr.New(t)})
			if err == nil || err.Error() != tc.expectedError.Error() {
				t.Fatalf("expected error %v, got %v", tc.expectedError, err)
			}
		})
	}
}
