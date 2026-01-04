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
	"fmt"
	"log/slog"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/steptaker"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/trigger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/sys/pciids"
)

// Config defines the node-agent settings.
type Config struct {
	NodeName      string
	SysRoot       string
	OSReleasePath string
	PCIIDsPaths   []string
	KubeConfig    *rest.Config
}

// Agent reconciles PhysicalGPU objects based on local PCI scan.
type Agent struct {
	cfg   Config
	log   *log.Logger
	steps steptaker.StepTakers[state.State]
}

const eventQuietPeriod = time.Second

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
	steps := handler.NewSteps(
		log,
		handler.NewDiscoverHandler(pci, hostInfo),
		handler.NewApplyHandler(store),
		handler.NewCleanupHandler(store),
	)

	return &Agent{
		cfg:   cfg,
		log:   log,
		steps: steps,
	}
}

// Run starts the event-driven sync loop.
func (a *Agent) Run(ctx context.Context) error {
	if a.cfg.KubeConfig == nil {
		return fmt.Errorf("kube config is required")
	}

	notifyCh := make(chan struct{}, 1)
	notify := func() {
		select {
		case notifyCh <- struct{}{}:
		default:
		}
	}

	dyn, err := dynamic.NewForConfig(a.cfg.KubeConfig)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	sources := []trigger.Source{
		trigger.NewUdevPCI(a.log),
		trigger.NewPhysicalGPUWatcher(dyn, a.cfg.NodeName, a.log),
	}

	errCh := make(chan error, len(sources))
	for _, source := range sources {
		source := source
		go func() {
			if err := source.Run(ctx, notify); err != nil {
				errCh <- err
			}
		}()
	}

	timer := time.NewTimer(eventQuietPeriod)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		case <-notifyCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(eventQuietPeriod)
		case <-timer.C:
			if err := a.sync(ctx); err != nil {
				a.log.Error("sync failed", logger.SlogErr(err))
				notify()
			}
		}
	}
}

func (a *Agent) sync(ctx context.Context) error {
	ctx = logger.ToContext(ctx, slog.Default())
	st := state.New(a.cfg.NodeName)
	if _, err := a.steps.Run(ctx, st); err != nil {
		return err
	}
	a.log.Info("sync completed", "devices", len(st.Devices()))
	return nil
}
