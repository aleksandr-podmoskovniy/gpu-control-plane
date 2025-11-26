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
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// System represents controller-wide configuration loaded from the config file.
type System struct {
	Controllers    ControllersConfig    `json:"controllers" yaml:"controllers"`
	LeaderElection LeaderElectionConfig `json:"leaderElection" yaml:"leaderElection"`
	Module         ModuleSettings       `json:"module" yaml:"module"`
}

// ControllersConfig holds per-controller tuning knobs.
type ControllersConfig struct {
	GPUInventory ControllerConfig `json:"gpuInventory" yaml:"gpuInventory"`
	GPUBootstrap ControllerConfig `json:"gpuBootstrap" yaml:"gpuBootstrap"`
	GPUPool      ControllerConfig `json:"gpuPool" yaml:"gpuPool"`
}

// ControllerConfig currently exposes only worker concurrency but can be extended later.
type ControllerConfig struct {
	Workers      int           `json:"workers" yaml:"workers"`
	ResyncPeriod time.Duration `json:"resyncPeriod" yaml:"resyncPeriod"`
}

// LeaderElectionConfig describes controller-runtime leader election settings.
type LeaderElectionConfig struct {
	Enabled      bool   `json:"enabled" yaml:"enabled"`
	Namespace    string `json:"namespace" yaml:"namespace"`
	ID           string `json:"id" yaml:"id"`
	ResourceLock string `json:"resourceLock" yaml:"resourceLock"`
}

// DeviceApprovalMode describes how newly detected devices should be approved.
type DeviceApprovalMode string

const (
	DeviceApprovalModeManual    DeviceApprovalMode = "Manual"
	DeviceApprovalModeAutomatic DeviceApprovalMode = "Automatic"
	DeviceApprovalModeSelector  DeviceApprovalMode = "Selector"
)

// ModuleSettings holds high-level module policies delivered via ModuleConfig.
type ModuleSettings struct {
	ManagedNodes     ManagedNodesSettings   `json:"managedNodes" yaml:"managedNodes"`
	DeviceApproval   DeviceApprovalSettings `json:"deviceApproval" yaml:"deviceApproval"`
	Scheduling       SchedulingSettings     `json:"scheduling" yaml:"scheduling"`
	Placement        PlacementSettings      `json:"placement" yaml:"placement"`
	Monitoring       MonitoringSettings     `json:"monitoring" yaml:"monitoring"`
	Inventory        InventorySettings      `json:"inventory" yaml:"inventory"`
	HTTPS            HTTPSSettings          `json:"https" yaml:"https"`
	HighAvailability *bool                  `json:"highAvailability,omitempty" yaml:"highAvailability,omitempty"`
}

// ManagedNodesSettings controls which nodes are considered managed by default.
type ManagedNodesSettings struct {
	LabelKey         string `json:"labelKey" yaml:"labelKey"`
	EnabledByDefault bool   `json:"enabledByDefault" yaml:"enabledByDefault"`
}

// DeviceApprovalSettings controls default approval workflow for new devices.
type DeviceApprovalSettings struct {
	Mode     DeviceApprovalMode    `json:"mode" yaml:"mode"`
	Selector *metav1.LabelSelector `json:"selector,omitempty" yaml:"selector,omitempty"`
}

// SchedulingSettings contains default scheduling hints reused across controllers.
type SchedulingSettings struct {
	DefaultStrategy string `json:"defaultStrategy" yaml:"defaultStrategy"`
	TopologyKey     string `json:"topologyKey,omitempty" yaml:"topologyKey,omitempty"`
}

// PlacementSettings carries cluster-wide toleration knobs.
type PlacementSettings struct {
	CustomTolerationKeys []string `json:"customTolerationKeys,omitempty" yaml:"customTolerationKeys,omitempty"`
}

type MonitoringSettings struct {
	ServiceMonitor bool `json:"serviceMonitor" yaml:"serviceMonitor"`
}

type InventorySettings struct {
	ResyncPeriod string `json:"resyncPeriod" yaml:"resyncPeriod"`
}

type HTTPSMode string

const (
	HTTPSModeDisabled          HTTPSMode = "Disabled"
	HTTPSModeCertManager       HTTPSMode = "CertManager"
	HTTPSModeCustomCertificate HTTPSMode = "CustomCertificate"
	HTTPSModeOnlyInURI         HTTPSMode = "OnlyInURI"
)

type HTTPSSettings struct {
	Mode                    HTTPSMode `json:"mode" yaml:"mode"`
	CertManagerIssuer       string    `json:"certManagerIssuer" yaml:"certManagerIssuer"`
	CustomCertificateSecret string    `json:"customCertificateSecret,omitempty" yaml:"customCertificateSecret,omitempty"`
}

const (
	DefaultLeaderElectionID           = "gpu-control-plane-controller-leader-election"
	DefaultLeaderElectionResourceLock = "leases"
	defaultControllerWorkers          = 1
	defaultControllerResyncPeriod     = 30 * time.Second

	defaultManagedNodeLabelKey    = "gpu.deckhouse.io/enabled"
	defaultSchedulingStrategy     = "Spread"
	defaultSchedulingTopologyKey  = "topology.kubernetes.io/zone"
	defaultMonitoringService      = true
	defaultInventoryResyncPeriod  = "30s"
	defaultHTTPSMode              = HTTPSModeCertManager
	defaultHTTPSCertManagerIssuer = "letsencrypt"
)

// DefaultSystem returns a System configuration populated with safe defaults.
func DefaultSystem() System {
	return System{
		Controllers: ControllersConfig{
			GPUInventory: defaultControllerConfig(),
			GPUBootstrap: defaultControllerConfig(),
			GPUPool:      defaultControllerConfig(),
		},
		Module: ModuleSettings{
			ManagedNodes: ManagedNodesSettings{
				LabelKey:         defaultManagedNodeLabelKey,
				EnabledByDefault: true,
			},
			DeviceApproval: DeviceApprovalSettings{
				Mode: DeviceApprovalModeManual,
			},
			Scheduling: SchedulingSettings{
				DefaultStrategy: defaultSchedulingStrategy,
				TopologyKey:     defaultSchedulingTopologyKey,
			},
			Monitoring: MonitoringSettings{
				ServiceMonitor: defaultMonitoringService,
			},
			Inventory: InventorySettings{
				ResyncPeriod: defaultInventoryResyncPeriod,
			},
			HTTPS: HTTPSSettings{
				Mode:              defaultHTTPSMode,
				CertManagerIssuer: defaultHTTPSCertManagerIssuer,
			},
		},
		LeaderElection: LeaderElectionConfig{
			Enabled:      false,
			ID:           DefaultLeaderElectionID,
			ResourceLock: DefaultLeaderElectionResourceLock,
		},
	}
}

func defaultControllerConfig() ControllerConfig {
	return ControllerConfig{
		Workers:      defaultControllerWorkers,
		ResyncPeriod: defaultControllerResyncPeriod,
	}
}

// LoadFile reads the YAML configuration file and merges it with defaults.
func LoadFile(path string) (System, error) {
	cfg := DefaultSystem()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}

	normalizeControllerWorkers(&cfg.Controllers.GPUInventory)
	normalizeControllerWorkers(&cfg.Controllers.GPUBootstrap)
	normalizeControllerWorkers(&cfg.Controllers.GPUPool)
	normalizeControllerResync(&cfg.Controllers.GPUInventory)
	normalizeControllerResync(&cfg.Controllers.GPUBootstrap)
	normalizeControllerResync(&cfg.Controllers.GPUPool)
	normalizeLeaderElection(&cfg.LeaderElection)
	normalizeModuleSettings(&cfg.Module)

	return cfg, nil
}

func normalizeControllerWorkers(cfg *ControllerConfig) {
	if cfg.Workers <= 0 {
		cfg.Workers = defaultControllerWorkers
	}
}

func normalizeControllerResync(cfg *ControllerConfig) {
	if cfg.ResyncPeriod <= 0 {
		cfg.ResyncPeriod = defaultControllerResyncPeriod
	}
}

func normalizeLeaderElection(cfg *LeaderElectionConfig) {
	if strings.TrimSpace(cfg.ID) == "" {
		cfg.ID = DefaultLeaderElectionID
	}
	if strings.TrimSpace(cfg.ResourceLock) == "" {
		cfg.ResourceLock = DefaultLeaderElectionResourceLock
	}
	cfg.Namespace = strings.TrimSpace(cfg.Namespace)
}

func normalizeModuleSettings(cfg *ModuleSettings) {
	cfg.ManagedNodes.LabelKey = strings.TrimSpace(cfg.ManagedNodes.LabelKey)
	if cfg.ManagedNodes.LabelKey == "" {
		cfg.ManagedNodes.LabelKey = defaultManagedNodeLabelKey
	}

	switch cfg.DeviceApproval.Mode {
	case DeviceApprovalModeAutomatic, DeviceApprovalModeSelector, DeviceApprovalModeManual:
		// valid value, keep as-is
	default:
		cfg.DeviceApproval.Mode = DeviceApprovalModeManual
	}
	if cfg.DeviceApproval.Mode == DeviceApprovalModeSelector && cfg.DeviceApproval.Selector == nil {
		cfg.DeviceApproval.Selector = &metav1.LabelSelector{}
	}

	cfg.Scheduling.DefaultStrategy = strings.TrimSpace(cfg.Scheduling.DefaultStrategy)
	if cfg.Scheduling.DefaultStrategy == "" {
		cfg.Scheduling.DefaultStrategy = defaultSchedulingStrategy
	}
	switch strings.ToLower(cfg.Scheduling.DefaultStrategy) {
	case "spread":
		cfg.Scheduling.DefaultStrategy = defaultSchedulingStrategy
	case "binpack":
		cfg.Scheduling.DefaultStrategy = "BinPack"
	default:
		cfg.Scheduling.DefaultStrategy = defaultSchedulingStrategy
	}

	cfg.Scheduling.TopologyKey = strings.TrimSpace(cfg.Scheduling.TopologyKey)
	if cfg.Scheduling.DefaultStrategy == "Spread" && cfg.Scheduling.TopologyKey == "" {
		cfg.Scheduling.TopologyKey = defaultSchedulingTopologyKey
	}

	seen := map[string]struct{}{}
	keys := make([]string, 0, len(cfg.Placement.CustomTolerationKeys))
	for _, k := range cfg.Placement.CustomTolerationKeys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	cfg.Placement.CustomTolerationKeys = keys

	cfg.Inventory.ResyncPeriod = strings.TrimSpace(cfg.Inventory.ResyncPeriod)
	if cfg.Inventory.ResyncPeriod == "" {
		cfg.Inventory.ResyncPeriod = defaultInventoryResyncPeriod
	}

	switch cfg.HTTPS.Mode {
	case HTTPSModeDisabled, HTTPSModeCertManager, HTTPSModeCustomCertificate, HTTPSModeOnlyInURI:
	default:
		cfg.HTTPS.Mode = defaultHTTPSMode
	}
	cfg.HTTPS.CertManagerIssuer = strings.TrimSpace(cfg.HTTPS.CertManagerIssuer)
	if cfg.HTTPS.CertManagerIssuer == "" {
		cfg.HTTPS.CertManagerIssuer = defaultHTTPSCertManagerIssuer
	}
	cfg.HTTPS.CustomCertificateSecret = strings.TrimSpace(cfg.HTTPS.CustomCertificateSecret)
	if cfg.HTTPS.Mode != HTTPSModeCustomCertificate {
		cfg.HTTPS.CustomCertificateSecret = ""
	}
}
