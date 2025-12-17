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

package manager

import (
	"context"
	"testing"

	"k8s.io/client-go/rest"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

func TestRunUsesProvidedRestConfig(t *testing.T) {
	origNewManager := newManager
	origSetupControllers := setupControllers
	origGetConfig := getConfigOrDie

	t.Cleanup(func() {
		newManager = origNewManager
		setupControllers = origSetupControllers
		getConfigOrDie = origGetConfig
	})

	cfg := &rest.Config{}
	capturedCfg := (*rest.Config)(nil)
	newManager = func(rc *rest.Config, opts ctrlmanager.Options) (ctrlmanager.Manager, error) {
		capturedCfg = rc
		return newFakeManager(), nil
	}
	setupControllers = func(context.Context, ctrlmanager.Manager, config.ControllersConfig, *moduleconfig.ModuleConfigStore) error {
		return nil
	}
	getConfigOrDie = func() *rest.Config {
		t.Fatalf("getConfigOrDie must not be called when restCfg provided")
		return nil
	}

	if err := Run(context.Background(), cfg, config.DefaultSystem()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if capturedCfg != cfg {
		t.Fatalf("expected provided rest config to be used")
	}
}
