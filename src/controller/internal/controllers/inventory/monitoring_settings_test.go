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

	config "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	moduleconfigpkg "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
)

func TestMonitoringEnabledDefaultsToTrue(t *testing.T) {
	rec := &Reconciler{}
	if !rec.monitoringEnabled() {
		t.Fatalf("expected monitoring to be enabled by default without store")
	}
}

func TestMonitoringEnabledRespectsStore(t *testing.T) {
	state := moduleconfigpkg.DefaultState()
	falseVal := false
	state.Settings.Monitoring.ServiceMonitor = false
	state.Sanitized["monitoring"].(map[string]any)["serviceMonitor"] = falseVal

	store := config.NewModuleConfigStore(state)
	rec := &Reconciler{store: store, fallbackMonitoring: true}
	if rec.monitoringEnabled() {
		t.Fatalf("expected monitoring to be disabled when ModuleConfig says so")
	}
}

func TestMonitoringEnabledFallback(t *testing.T) {
	rec := &Reconciler{fallbackMonitoring: true}
	if !rec.monitoringEnabled() {
		t.Fatalf("expected monitoring enabled when fallbackMonitoring is true")
	}
}
