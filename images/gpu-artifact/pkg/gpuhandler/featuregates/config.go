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
	"k8s.io/client-go/kubernetes"

	k8sresourceslice "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/k8s/resourceslice"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

func (s *Service) ConfigureConsumableCapacity(kubeClient kubernetes.Interface, modeRaw string) {
	mode, err := parseConsumableCapacityMode(modeRaw)
	if err != nil && s.log != nil {
		s.log.Warn("unsupported consumable capacity mode, falling back to auto", "value", modeRaw, logger.SlogErr(err))
	}

	enabled, source, serverVersion, resolveErr := resolveConsumableCapacity(kubeClient, mode)
	if resolveErr != nil && s.log != nil {
		s.log.Warn("failed to resolve consumable capacity mode", "mode", mode, "source", source, "apiserverVersion", serverVersion, logger.SlogErr(resolveErr))
	}

	if enabled && s.builder != nil {
		s.builder.EnableFeatures([]string{featureConsumableCapacity})
	}

	if s.log != nil {
		s.log.Info("consumable capacity mode resolved", "mode", mode, "enabled", enabled, "source", source, "apiserverVersion", serverVersion)
	}
}

func (s *Service) ConfigureSharedCountersLayout(kubeClient kubernetes.Interface) {
	layout, source, serverVersion, err := resolveSharedCountersLayout(kubeClient)
	if err != nil && s.log != nil {
		s.log.Warn("failed to resolve shared counters layout", "source", source, "apiserverVersion", serverVersion, logger.SlogErr(err))
	}

	if s.builder != nil {
		s.builder.SetSharedCountersLayout(layout)
	}

	if s.log != nil {
		s.log.Info("partitionable shared counters layout resolved", "layout", sharedCountersLayoutLabel(layout), "source", source, "apiserverVersion", serverVersion)
	}
}

func sharedCountersLayoutLabel(layout k8sresourceslice.SharedCountersLayout) string {
	switch layout {
	case k8sresourceslice.SharedCountersSeparate:
		return "separate"
	default:
		return "inline"
	}
}
