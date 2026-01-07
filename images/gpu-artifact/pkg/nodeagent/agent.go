/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodeagent

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/steptaker"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/sys/pciids"
)

// Agent reconciles PhysicalGPU objects based on local PCI scan.
type Agent struct {
	cfg   Config
	log   *log.Logger
	steps steptaker.StepTakers[state.State]

	scheme   *runtime.Scheme
	store    service.Store
	pci      service.PCIProvider
	hostInfo service.HostInfoProvider
}

// New creates a new node-agent.
func New(client client.Client, cfg Config, log *log.Logger) *Agent {
	store := service.NewClientStore(client)
	resolver, usedPath, err := pciids.LoadFirst(cfg.PCIIDsPaths)
	if err != nil {
		log.Error("failed to load pci.ids", logger.SlogErr(err))
	}
	if resolver == nil {
		log.Info("pci.ids not found, PCI names will be empty", "paths", cfg.PCIIDsPaths)
	} else {
		log.Info("pci.ids loaded", "path", usedPath)
	}

	pci := service.NewSysfsPCIProvider(cfg.SysRoot, resolver)
	hostInfo := service.NewHostInfoCollector(cfg.OSReleasePath, cfg.SysRoot)

	return &Agent{
		cfg:      cfg,
		log:      log,
		scheme:   client.Scheme(),
		store:    store,
		pci:      pci,
		hostInfo: hostInfo,
	}
}
