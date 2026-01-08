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

package apply

import (
	"context"
	"log/slog"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

func (h *ApplyHandler) recordEvent(log *slog.Logger, obj *gpuv1alpha1.PhysicalGPU, eventType, reason, message string) {
	if h.recorder == nil || obj == nil {
		return
	}
	if log == nil {
		h.recorder.Event(obj, eventType, reason, message)
		return
	}
	h.recorder.WithLogging(log).Event(obj, eventType, reason, message)
}

func deviceLog(ctx context.Context, nodeName, name string, dev state.Device) *slog.Logger {
	log := logger.FromContext(ctx).With("node", nodeName, "physicalgpu", name)
	if dev.Address != "" {
		log = log.With("pci", dev.Address)
	}
	if vendor := state.VendorLabel(dev); vendor != "" {
		log = log.With("vendor", vendor)
	}
	if device := state.DeviceLabel(dev.DeviceName); device != "" {
		log = log.With("device", device)
	}
	return log
}
