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

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/steptaker"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/driver"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/capabilities"
	inventorysvc "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
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
