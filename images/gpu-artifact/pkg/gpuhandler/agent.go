//go:build linux && cgo && nvml
// +build linux,cgo,nvml

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

package gpuhandler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/steptaker"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/driver"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/resourceslice"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/trigger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

// Config defines the gpu-handler settings.
type Config struct {
	NodeName   string
	KubeConfig *rest.Config
}

// Agent reconciles PhysicalGPU objects for a single node.
type Agent struct {
	cfg Config
	log *log.Logger

	store          *service.PhysicalGPUService
	reader         service.CapabilitiesReader
	placements     inventory.MigPlacementReader
	tracker        handler.FailureTracker
	steps          steptaker.StepTakers[state.State]
	draDriver      *driver.Driver
	draDriverReady bool
}

const eventQuietPeriod = time.Second
const heartbeatPeriod = 60 * time.Second

// New creates a new gpu-handler agent.
func New(client client.Client, cfg Config, log *log.Logger) *Agent {
	store := service.NewPhysicalGPUService(client)
	nvmlService := service.NewNVML()
	reader := service.NewNVMLReader(nvmlService)
	placements := inventory.NewNVMLMigPlacementReader(nvmlService)
	tracker := state.NewNVMLFailureTracker(nil)

	return &Agent{
		cfg:        cfg,
		log:        log,
		store:      store,
		reader:     reader,
		placements: placements,
		tracker:    tracker,
	}
}

// Run starts the event-driven sync loop.
func (a *Agent) Run(ctx context.Context) error {
	if a.cfg.KubeConfig == nil {
		return fmt.Errorf("kube config is required")
	}

	if err := a.startDRA(ctx); err != nil {
		return err
	}
	defer a.stopDRA()

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
	heartbeat := time.NewTicker(heartbeatPeriod)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		case <-heartbeat.C:
			notify()
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
	if !a.draDriverReady {
		return fmt.Errorf("DRA driver is not started")
	}

	st := state.New(a.cfg.NodeName)
	if _, err := a.steps.Run(ctx, st); err != nil {
		return err
	}
	a.log.Info("sync completed", "all", len(st.All()), "ready", len(st.Ready()))
	return nil
}

func (a *Agent) startDRA(ctx context.Context) error {
	kubeClient, err := kubernetes.NewForConfig(a.cfg.KubeConfig)
	if err != nil {
		return fmt.Errorf("create kube clientset: %w", err)
	}

	draDriver, err := driver.Start(ctx, driver.Config{
		NodeName:   a.cfg.NodeName,
		KubeClient: kubeClient,
	})
	if err != nil {
		return fmt.Errorf("start DRA driver: %w", err)
	}

	a.draDriver = draDriver
	a.draDriverReady = true
	a.steps = handler.NewSteps(
		a.log,
		handler.NewDiscoverHandler(a.store),
		handler.NewMarkNotReadyHandler(a.store, a.tracker),
		handler.NewFilterReadyHandler(),
		handler.NewCapabilitiesHandler(a.reader, a.store, a.tracker),
		handler.NewFilterHealthyHandler(),
		handler.NewPublishResourcesHandler(resourceslice.NewBuilder(a.placements), a.draDriver),
	)
	return nil
}

func (a *Agent) stopDRA() {
	if a.draDriver != nil {
		a.draDriver.Shutdown()
	}
}
