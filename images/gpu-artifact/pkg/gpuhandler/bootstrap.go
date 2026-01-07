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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/steptaker"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/driver"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/featuregates"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler/health"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler/inventory"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler/publish"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/capabilities"
	inventorysvc "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory"
	handlerresourceslice "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/resourceslice"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

type bootstrapService struct {
	cfg        Config
	log        *log.Logger
	scheme     *runtime.Scheme
	store      *service.PhysicalGPUService
	reader     capabilities.CapabilitiesReader
	placements inventorysvc.MigPlacementReader
	tracker    handler.FailureTracker
}

type bootstrapResult struct {
	driver   *driver.Driver
	steps    steptaker.StepTakers[state.State]
	stop     func()
	recorder eventrecord.EventRecorderLogger
}

func newBootstrapService(cfg Config, log *log.Logger, scheme *runtime.Scheme, store *service.PhysicalGPUService, reader capabilities.CapabilitiesReader, placements inventorysvc.MigPlacementReader, tracker handler.FailureTracker) *bootstrapService {
	return &bootstrapService{
		cfg:        cfg,
		log:        log,
		scheme:     scheme,
		store:      store,
		reader:     reader,
		placements: placements,
		tracker:    tracker,
	}
}

func (b *bootstrapService) Start(ctx context.Context, notify func()) (*bootstrapResult, error) {
	kubeClient, err := kubernetes.NewForConfig(b.cfg.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("create kube clientset: %w", err)
	}

	builder := handlerresourceslice.NewBuilder(b.placements)
	recorder, stopRecorder := newEventRecorder(kubeClient, b.scheme, handlerComponent)
	if recorder != nil {
		recorder = recorder.WithLogging(b.log.With(logger.SlogController(handlerComponent)))
	}

	featureGates := featuregates.NewService(b.cfg.NodeName, b.store, b.log, builder, recorder, notify)
	featureGates.ConfigureConsumableCapacity(kubeClient, b.cfg.ConsumableCapacityMode)
	featureGates.ConfigureSharedCountersLayout(kubeClient)

	draDriver, err := driver.Start(ctx, driver.Config{
		NodeName:          b.cfg.NodeName,
		KubeClient:        kubeClient,
		DriverRoot:        b.cfg.DriverRoot,
		HostDriverRoot:    b.cfg.HostDriverRoot,
		CDIRoot:           b.cfg.CDIRoot,
		NvidiaCDIHookPath: b.cfg.NvidiaCDIHookPath,
		ErrorHandler:      featureGates.HandleError,
	})
	if err != nil {
		if stopRecorder != nil {
			stopRecorder()
		}
		return nil, fmt.Errorf("start DRA driver: %w", err)
	}

	steps := handler.NewSteps(
		b.log,
		inventory.NewDiscoverHandler(b.store),
		health.NewMarkNotReadyHandler(b.store, b.tracker, recorder),
		inventory.NewFilterReadyHandler(),
		health.NewCapabilitiesHandler(b.reader, b.store, b.tracker, recorder),
		health.NewFilterHealthyHandler(),
		publish.NewPublishResourcesHandler(builder, draDriver, recorder, featureGates.HandleError),
	)

	stop := func() {
		draDriver.Shutdown()
		if stopRecorder != nil {
			stopRecorder()
		}
	}

	return &bootstrapResult{
		driver:   draDriver,
		steps:    steps,
		stop:     stop,
		recorder: recorder,
	}, nil
}
