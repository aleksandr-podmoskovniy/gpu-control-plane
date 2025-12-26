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
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/sys/pciids"
)

// Config defines the node-agent settings.
type Config struct {
	NodeName      string
	SysRoot       string
	OSReleasePath string
	PCIIDsPaths   []string
	ResyncPeriod  time.Duration
}

// Agent reconciles PhysicalGPU objects based on local PCI scan.
type Agent struct {
	cfg   Config
	log   *log.Logger
	chain handler.Chain
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
	chain := handler.NewChain(
		handler.NewDiscoverHandler(pci, hostInfo),
		handler.NewApplyHandler(store),
		handler.NewCleanupHandler(store),
	)

	return &Agent{
		cfg:   cfg,
		log:   log,
		chain: chain,
	}
}

// Run starts the periodic sync loop.
func (a *Agent) Run(ctx context.Context) error {
	if err := a.sync(ctx); err != nil {
		a.log.Error("initial sync failed", logger.SlogErr(err))
	}

	period := a.cfg.ResyncPeriod
	if period <= 0 {
		<-ctx.Done()
		return nil
	}

	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := a.sync(ctx); err != nil {
				a.log.Error("sync failed", logger.SlogErr(err))
			}
		}
	}
}

func (a *Agent) sync(ctx context.Context) error {
	st := state.New(a.cfg.NodeName)
	if err := a.chain.Run(ctx, st, a.log); err != nil {
		return err
	}
	a.log.Info("sync completed", "devices", len(st.Devices()))
	return nil
}
