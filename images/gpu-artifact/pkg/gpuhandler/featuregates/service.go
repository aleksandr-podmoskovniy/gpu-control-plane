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

package featuregates

import (
	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service"
	handlerresourceslice "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/resourceslice"
)

// Service handles DRA feature gates and related events.
type Service struct {
	nodeName string
	store    *service.PhysicalGPUService
	log      *log.Logger
	recorder eventrecord.EventRecorderLogger
	builder  *handlerresourceslice.Builder
	tracker  *featureGateTracker
	notify   func()
}

// NewService creates a feature-gate handler service.
func NewService(nodeName string, store *service.PhysicalGPUService, log *log.Logger, builder *handlerresourceslice.Builder, recorder eventrecord.EventRecorderLogger, notify func()) *Service {
	return &Service{
		nodeName: nodeName,
		store:    store,
		log:      log,
		recorder: recorder,
		builder:  builder,
		tracker:  newFeatureGateTracker(),
		notify:   notify,
	}
}
