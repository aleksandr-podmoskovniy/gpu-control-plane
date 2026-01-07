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

package health

import (
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/capabilities"
)

const capabilitiesHandlerName = "capabilities"

// CapabilitiesHandler enriches PhysicalGPU status using NVML.
type CapabilitiesHandler struct {
	reader   capabilities.CapabilitiesReader
	store    *service.PhysicalGPUService
	tracker  handler.FailureTracker
	recorder eventrecord.EventRecorderLogger
}

// NewCapabilitiesHandler constructs a capabilities handler.
func NewCapabilitiesHandler(reader capabilities.CapabilitiesReader, store *service.PhysicalGPUService, tracker handler.FailureTracker, recorder eventrecord.EventRecorderLogger) *CapabilitiesHandler {
	return &CapabilitiesHandler{
		reader:   reader,
		store:    store,
		tracker:  tracker,
		recorder: recorder,
	}
}

// Name returns the handler name.
func (h *CapabilitiesHandler) Name() string {
	return capabilitiesHandlerName
}
