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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/steptaker"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/driver"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/capabilities"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory"
	nvmlsvc "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/nvml"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
)

// Agent reconciles PhysicalGPU objects for a single node.
type Agent struct {
	cfg Config
	log *log.Logger

	scheme     *runtime.Scheme
	store      *service.PhysicalGPUService
	reader     capabilities.CapabilitiesReader
	placements inventory.MigPlacementReader
	tracker    handler.FailureTracker
	steps      steptaker.StepTakers[state.State]
	draDriver  *driver.Driver
	stop       func()
}

// New creates a new gpu-handler agent.
func New(client client.Client, cfg Config, log *log.Logger) *Agent {
	store := service.NewPhysicalGPUService(client)
	nvmlService := nvmlsvc.NewNVML()
	reader := capabilities.NewNVMLReader(nvmlService)
	placements := inventory.NewNVMLMigPlacementReader(nvmlService)
	tracker := state.NewNVMLFailureTracker(nil)

	return &Agent{
		cfg:        cfg,
		log:        log,
		scheme:     client.Scheme(),
		store:      store,
		reader:     reader,
		placements: placements,
		tracker:    tracker,
	}
}
