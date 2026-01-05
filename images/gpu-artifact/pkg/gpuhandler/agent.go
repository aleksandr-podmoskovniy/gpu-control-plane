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
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	dresourceslice "k8s.io/dynamic-resource-allocation/resourceslice"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/steptaker"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/driver"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory"
	handlerresourceslice "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/resourceslice"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/trigger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

// Config defines the gpu-handler settings.
type Config struct {
	NodeName                string
	KubeConfig              *rest.Config
	ConsumableCapacityMode  string
}

// Agent reconciles PhysicalGPU objects for a single node.
type Agent struct {
	cfg Config
	log *log.Logger

	scheme          *runtime.Scheme
	store           *service.PhysicalGPUService
	reader          service.CapabilitiesReader
	placements      inventory.MigPlacementReader
	tracker         handler.FailureTracker
	steps           steptaker.StepTakers[state.State]
	draDriver       *driver.Driver
	draDriverReady  bool
	resourceBuilder *handlerresourceslice.Builder
	recorder        eventrecord.EventRecorderLogger
	stopRecorder    func()
	featureGates    *featureGateTracker
	notify          func()
	consumableCapacityEnabled bool
}

const eventQuietPeriod = time.Second
const heartbeatPeriod = 60 * time.Second

const (
	handlerComponent          = "gpu-handler"
	reasonFeatureGateDisabled = "FeatureGateDisabled"
	reasonExclusiveFallback   = "ExclusiveFallback"
	featurePartitionable      = "DRAPartitionableDevices"
	featureConsumableCapacity = "DRAConsumableCapacity"
)

// New creates a new gpu-handler agent.
func New(client client.Client, cfg Config, log *log.Logger) *Agent {
	store := service.NewPhysicalGPUService(client)
	nvmlService := service.NewNVML()
	reader := service.NewNVMLReader(nvmlService)
	placements := inventory.NewNVMLMigPlacementReader(nvmlService)
	tracker := state.NewNVMLFailureTracker(nil)

	return &Agent{
		cfg:          cfg,
		log:          log,
		scheme:       client.Scheme(),
		store:        store,
		reader:       reader,
		placements:   placements,
		tracker:      tracker,
		featureGates: newFeatureGateTracker(),
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
	a.notify = notify

	if err := a.startDRA(ctx); err != nil {
		return err
	}
	defer a.stopDRA()

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

	builder := handlerresourceslice.NewBuilder(a.placements)
	a.resourceBuilder = builder
	a.startEventRecorder(kubeClient)

	a.configureConsumableCapacity(kubeClient, builder)

	draDriver, err := driver.Start(ctx, driver.Config{
		NodeName:     a.cfg.NodeName,
		KubeClient:   kubeClient,
		ErrorHandler: a.handleDRAError,
	})
	if err != nil {
		return fmt.Errorf("start DRA driver: %w", err)
	}

	a.draDriver = draDriver
	a.draDriverReady = true
	a.steps = handler.NewSteps(
		a.log,
		handler.NewDiscoverHandler(a.store),
		handler.NewMarkNotReadyHandler(a.store, a.tracker, a.recorder),
		handler.NewFilterReadyHandler(),
		handler.NewCapabilitiesHandler(a.reader, a.store, a.tracker, a.recorder),
		handler.NewFilterHealthyHandler(),
		handler.NewPublishResourcesHandler(builder, a.draDriver, a.recorder),
	)
	return nil
}

func (a *Agent) stopDRA() {
	if a.draDriver != nil {
		a.draDriver.Shutdown()
	}
	if a.stopRecorder != nil {
		a.stopRecorder()
	}
}

func (a *Agent) startEventRecorder(kubeClient kubernetes.Interface) {
	if a.recorder != nil || a.scheme == nil {
		return
	}
	recorder, stop := newEventRecorder(kubeClient, a.scheme, handlerComponent)
	if recorder == nil {
		return
	}
	a.recorder = recorder.WithLogging(a.log.With(logger.SlogController(handlerComponent)))
	a.stopRecorder = stop
}

func (a *Agent) handleDRAError(ctx context.Context, err error, msg string) {
	var dropped *dresourceslice.DroppedFieldsError
	if errors.As(err, &dropped) {
		disabled := dropped.DisabledFeatures()
		if len(disabled) == 0 {
			a.log.Warn("DRA fields dropped without detected feature gate", "pool", dropped.PoolName, "sliceIndex", dropped.SliceIndex)
			utilruntime.HandleErrorWithContext(ctx, err, msg)
			return
		}

		known, unknown := splitFeatures(disabled)
		if len(known) > 0 && a.resourceBuilder != nil {
			if a.resourceBuilder.DisableFeatures(known) {
				a.log.Warn("DRA features disabled after apiserver dropped fields", "features", known, "pool", dropped.PoolName, "sliceIndex", dropped.SliceIndex)
				if a.notify != nil {
					a.notify()
				}
			}
		}

		a.recordFeatureGateEvents(ctx, dropped.PoolName, known)
		if len(unknown) > 0 {
			a.log.Warn("DRA fields dropped for unsupported features", "features", unknown, "pool", dropped.PoolName, "sliceIndex", dropped.SliceIndex)
			utilruntime.HandleErrorWithContext(ctx, err, msg)
		}
		return
	}
	utilruntime.HandleErrorWithContext(ctx, err, msg)
}

func splitFeatures(features []string) ([]string, []string) {
	var known []string
	var unknown []string
	for _, feature := range features {
		switch feature {
		case featurePartitionable, featureConsumableCapacity:
			known = append(known, feature)
		default:
			unknown = append(unknown, feature)
		}
	}
	return known, unknown
}

func (a *Agent) configureConsumableCapacity(kubeClient kubernetes.Interface, builder *handlerresourceslice.Builder) {
	mode, err := parseConsumableCapacityMode(a.cfg.ConsumableCapacityMode)
	if err != nil {
		a.log.Warn("unsupported consumable capacity mode, falling back to auto", "value", a.cfg.ConsumableCapacityMode, logger.SlogErr(err))
	}

	enabled, source, serverVersion, resolveErr := resolveConsumableCapacity(kubeClient, mode)
	if resolveErr != nil {
		a.log.Warn("failed to resolve consumable capacity mode", "mode", mode, "source", source, "apiserverVersion", serverVersion, logger.SlogErr(resolveErr))
	}

	a.consumableCapacityEnabled = enabled
	if enabled && builder != nil {
		builder.EnableFeatures([]string{featureConsumableCapacity})
	}

	a.log.Info("consumable capacity mode resolved", "mode", mode, "enabled", enabled, "source", source, "apiserverVersion", serverVersion)
}

func (a *Agent) recordFeatureGateEvents(ctx context.Context, poolName string, features []string) {
	if a.recorder == nil || a.featureGates == nil || len(features) == 0 || a.store == nil {
		return
	}
	newlyDisabled := a.featureGates.MarkDisabled(features)
	if len(newlyDisabled) == 0 {
		return
	}

	nodeName := strings.TrimPrefix(poolName, "gpus/")
	if nodeName == "" || nodeName == poolName {
		nodeName = a.cfg.NodeName
	}

	gpus, err := a.store.ListByNode(ctx, nodeName)
	if err != nil || len(gpus) == 0 {
		a.log.Warn("unable to load PhysicalGPU for feature gate events", "node", nodeName, logger.SlogErr(err))
		return
	}

	log := logger.FromContext(ctx).With("node", nodeName)
	for _, feature := range newlyDisabled {
		msg := fmt.Sprintf("%s is disabled on apiserver", feature)
		for i := range gpus {
			a.recorder.WithLogging(log.With("featureGate", feature)).Event(
				&gpus[i],
				corev1.EventTypeWarning,
				reasonFeatureGateDisabled,
				msg,
			)
		}

		if feature == featurePartitionable {
			for i := range gpus {
				a.recorder.WithLogging(log.With("featureGate", feature)).Event(
					&gpus[i],
					corev1.EventTypeWarning,
					reasonExclusiveFallback,
					"publishing exclusive Physical offers only (no MIG profiles)",
				)
			}
		}
	}
}
