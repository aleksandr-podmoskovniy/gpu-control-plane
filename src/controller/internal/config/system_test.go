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

package config

import (
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLoadFileMergesDefaults(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("controllers:\n  gpuInventory:\n    workers: 3\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if cfg.Controllers.GPUInventory.Workers != 3 {
		t.Fatalf("expected override to 3, got %d", cfg.Controllers.GPUInventory.Workers)
	}
	if cfg.Controllers.GPUBootstrap.Workers != 1 {
		t.Fatalf("expected default 1 for bootstrap, got %d", cfg.Controllers.GPUBootstrap.Workers)
	}
	if cfg.Controllers.GPUInventory.ResyncPeriod != defaultControllerResyncPeriod {
		t.Fatalf("expected default resync period, got %s", cfg.Controllers.GPUInventory.ResyncPeriod)
	}
	if cfg.LeaderElection.Enabled {
		t.Fatalf("expected leader election to remain disabled by default")
	}
	if cfg.LeaderElection.ID != DefaultLeaderElectionID {
		t.Fatalf("unexpected default leader election ID: %s", cfg.LeaderElection.ID)
	}
	if cfg.LeaderElection.ResourceLock != DefaultLeaderElectionResourceLock {
		t.Fatalf("unexpected default leader election resource lock: %s", cfg.LeaderElection.ResourceLock)
	}
	if cfg.Module.ManagedNodes.LabelKey != defaultManagedNodeLabelKey {
		t.Fatalf("unexpected managed node label key default: %s", cfg.Module.ManagedNodes.LabelKey)
	}
	if !cfg.Module.ManagedNodes.EnabledByDefault {
		t.Fatalf("expected managed nodes enabled by default")
	}
	if cfg.Module.DeviceApproval.Mode != DeviceApprovalModeManual {
		t.Fatalf("unexpected default device approval mode: %s", cfg.Module.DeviceApproval.Mode)
	}
	if cfg.Module.Scheduling.DefaultStrategy != defaultSchedulingStrategy {
		t.Fatalf("unexpected default scheduling strategy: %s", cfg.Module.Scheduling.DefaultStrategy)
	}
	if cfg.Module.Scheduling.TopologyKey != defaultSchedulingTopologyKey {
		t.Fatalf("unexpected default topology key: %s", cfg.Module.Scheduling.TopologyKey)
	}
}

func TestLoadFileNormalisesWorkers(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("controllers:\n  admission:\n    workers: 0\n    resyncPeriod: 0s\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if cfg.Controllers.Admission.Workers != 1 {
		t.Fatalf("expected normalised to 1, got %d", cfg.Controllers.Admission.Workers)
	}
	if cfg.Controllers.Admission.ResyncPeriod != defaultControllerResyncPeriod {
		t.Fatalf("expected default resync period, got %s", cfg.Controllers.Admission.ResyncPeriod)
	}
}

func TestLoadFileLeaderElectionOverrides(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	payload := []byte(`leaderElection:
  enabled: true
  namespace: custom-ns
  id: custom-id
  resourceLock: endpointsleases
`)
	if err := os.WriteFile(cfgPath, payload, 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if !cfg.LeaderElection.Enabled {
		t.Fatalf("expected enabled leader election")
	}
	if cfg.LeaderElection.Namespace != "custom-ns" {
		t.Fatalf("unexpected namespace: %s", cfg.LeaderElection.Namespace)
	}
	if cfg.LeaderElection.ID != "custom-id" {
		t.Fatalf("unexpected id: %s", cfg.LeaderElection.ID)
	}
	if cfg.LeaderElection.ResourceLock != "endpointsleases" {
		t.Fatalf("unexpected resource lock: %s", cfg.LeaderElection.ResourceLock)
	}
	if cfg.Controllers.GPUInventory.ResyncPeriod != defaultControllerResyncPeriod {
		t.Fatalf("expected inventory resync period untouched, got %s", cfg.Controllers.GPUInventory.ResyncPeriod)
	}
}

func TestLoadFileMissingFile(t *testing.T) {
	if _, err := LoadFile("/non/existing/path.yaml"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadFileModuleOverrides(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	payload := []byte(`module:
  managedNodes:
    labelKey: gpu.deckhouse.io/custom-enabled
    enabledByDefault: false
  deviceApproval:
    mode: Selector
    selector:
      matchLabels:
        gpu.deckhouse.io/device.vendor: "10de"
  scheduling:
    defaultStrategy: BinPack
    topologyKey: topology.gpu.io/zone
`)
	if err := os.WriteFile(cfgPath, payload, 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if cfg.Module.ManagedNodes.LabelKey != "gpu.deckhouse.io/custom-enabled" {
		t.Fatalf("unexpected managed label key: %s", cfg.Module.ManagedNodes.LabelKey)
	}
	if cfg.Module.ManagedNodes.EnabledByDefault {
		t.Fatal("expected managed nodes default disabled")
	}
	if cfg.Module.DeviceApproval.Mode != DeviceApprovalModeSelector {
		t.Fatalf("unexpected approval mode: %s", cfg.Module.DeviceApproval.Mode)
	}
	if cfg.Module.DeviceApproval.Selector == nil {
		t.Fatalf("expected selector to be preserved")
	}
	if cfg.Module.Scheduling.DefaultStrategy != "BinPack" {
		t.Fatalf("unexpected scheduling strategy: %s", cfg.Module.Scheduling.DefaultStrategy)
	}
	if cfg.Module.Scheduling.TopologyKey != "topology.gpu.io/zone" {
		t.Fatalf("unexpected topology key: %s", cfg.Module.Scheduling.TopologyKey)
	}
}

func TestNormalizeLeaderElectionDefaults(t *testing.T) {
	cfg := LeaderElectionConfig{
		Enabled:      true,
		Namespace:    "  gpu-system  ",
		ID:           "   ",
		ResourceLock: "",
	}

	normalizeLeaderElection(&cfg)

	if cfg.ID != DefaultLeaderElectionID {
		t.Fatalf("expected default ID, got %s", cfg.ID)
	}
	if cfg.ResourceLock != DefaultLeaderElectionResourceLock {
		t.Fatalf("expected default resource lock, got %s", cfg.ResourceLock)
	}
	if cfg.Namespace != "gpu-system" {
		t.Fatalf("expected namespace trimmed, got %q", cfg.Namespace)
	}
}

func TestNormalizeModuleSettingsDefaults(t *testing.T) {
	cfg := ModuleSettings{
		ManagedNodes: ManagedNodesSettings{
			LabelKey:         "   ",
			EnabledByDefault: false,
		},
		DeviceApproval: DeviceApprovalSettings{
			Mode: "invalid",
		},
		Scheduling: SchedulingSettings{
			DefaultStrategy: "",
			TopologyKey:     "   ",
		},
	}

	normalizeModuleSettings(&cfg)

	if cfg.ManagedNodes.LabelKey != defaultManagedNodeLabelKey {
		t.Fatalf("expected managed label key default, got %s", cfg.ManagedNodes.LabelKey)
	}
	if cfg.ManagedNodes.EnabledByDefault {
		t.Fatal("expected managed nodes to remain false when explicitly disabled")
	}
	if cfg.DeviceApproval.Mode != DeviceApprovalModeManual {
		t.Fatalf("expected approval mode fallback to Manual, got %s", cfg.DeviceApproval.Mode)
	}
	if cfg.DeviceApproval.Selector != nil {
		t.Fatal("selector should remain nil for Manual mode")
	}
	if cfg.Scheduling.DefaultStrategy != defaultSchedulingStrategy {
		t.Fatalf("expected default scheduling strategy, got %s", cfg.Scheduling.DefaultStrategy)
	}
	if cfg.Scheduling.TopologyKey != defaultSchedulingTopologyKey {
		t.Fatalf("expected default topology key, got %s", cfg.Scheduling.TopologyKey)
	}
}

func TestNormalizeModuleSettingsSelectorMode(t *testing.T) {
	cfg := ModuleSettings{
		DeviceApproval: DeviceApprovalSettings{
			Mode:     DeviceApprovalModeSelector,
			Selector: nil,
		},
		Scheduling: SchedulingSettings{
			DefaultStrategy: "BinPack",
			TopologyKey:     " ignored ",
		},
	}

	normalizeModuleSettings(&cfg)

	if cfg.DeviceApproval.Selector == nil {
		t.Fatal("selector must be initialised when mode=Selector")
	}
	if cfg.Scheduling.DefaultStrategy != "BinPack" {
		t.Fatalf("expected BinPack to be preserved, got %s", cfg.Scheduling.DefaultStrategy)
	}
	if cfg.Scheduling.TopologyKey != "ignored" {
		t.Fatalf("expected topology key to be trimmed without forcing default, got %s", cfg.Scheduling.TopologyKey)
	}
}

func TestNormalizeModuleSettingsSelectorPreservesSelector(t *testing.T) {
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{"gpu.deckhouse.io/device.vendor": "10de"},
	}
	cfg := ModuleSettings{
		DeviceApproval: DeviceApprovalSettings{
			Mode:     DeviceApprovalModeSelector,
			Selector: selector,
		},
	}

	normalizeModuleSettings(&cfg)

	if cfg.DeviceApproval.Selector != selector {
		t.Fatal("existing selector should be preserved when provided")
	}
}

func TestNormalizeModuleSettingsAutomaticMode(t *testing.T) {
	cfg := ModuleSettings{
		DeviceApproval: DeviceApprovalSettings{
			Mode: DeviceApprovalModeAutomatic,
		},
	}

	normalizeModuleSettings(&cfg)

	if cfg.DeviceApproval.Mode != DeviceApprovalModeAutomatic {
		t.Fatalf("automatic mode must remain untouched, got %s", cfg.DeviceApproval.Mode)
	}
	if cfg.DeviceApproval.Selector != nil {
		t.Fatal("automatic mode should not populate selector")
	}
}

func TestNormalizeModuleSettingsUnknownStrategy(t *testing.T) {
	cfg := ModuleSettings{
		Scheduling: SchedulingSettings{
			DefaultStrategy: "something",
			TopologyKey:     "custom",
		},
	}

	normalizeModuleSettings(&cfg)

	if cfg.Scheduling.DefaultStrategy != defaultSchedulingStrategy {
		t.Fatalf("unexpected fallback strategy: %s", cfg.Scheduling.DefaultStrategy)
	}
	if cfg.Scheduling.TopologyKey != "custom" {
		t.Fatalf("topology key should remain trimmed when not spread, got %s", cfg.Scheduling.TopologyKey)
	}
}
func TestNormalizeModuleSettingsSpreadDefaultsTopology(t *testing.T) {
	cfg := ModuleSettings{
		Scheduling: SchedulingSettings{
			DefaultStrategy: "Spread",
			TopologyKey:     "   ",
		},
	}

	normalizeModuleSettings(&cfg)

	if cfg.Scheduling.TopologyKey != defaultSchedulingTopologyKey {
		t.Fatalf("expected topology key default for Spread strategy, got %s", cfg.Scheduling.TopologyKey)
	}
}

func TestLoadFileDecodeError(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "bad.yaml")
	if err := os.WriteFile(cfgPath, []byte("controllers: [invalid"), 0o600); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	if _, err := LoadFile(cfgPath); err == nil {
		t.Fatal("expected decode error for malformed yaml")
	}
}
