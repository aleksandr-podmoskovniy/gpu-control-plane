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
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

func (s *Service) recordFeatureGateEvents(ctx context.Context, poolName string, features []string) {
	if s.recorder == nil || s.tracker == nil || len(features) == 0 || s.store == nil {
		return
	}
	newlyDisabled := s.tracker.MarkDisabled(features)
	if len(newlyDisabled) == 0 {
		return
	}

	nodeName := strings.TrimPrefix(poolName, "gpus/")
	if nodeName == "" || nodeName == poolName {
		nodeName = s.nodeName
	}

	gpus, err := s.store.ListByNode(ctx, nodeName)
	if err != nil || len(gpus) == 0 {
		if s.log != nil {
			s.log.Warn("unable to load PhysicalGPU for feature gate events", "node", nodeName, logger.SlogErr(err))
		}
		return
	}

	log := logger.FromContext(ctx).With("node", nodeName)
	for _, feature := range newlyDisabled {
		msg := fmt.Sprintf("%s is disabled on apiserver", feature)
		for i := range gpus {
			s.recorder.WithLogging(log.With("featureGate", feature)).Event(
				&gpus[i],
				corev1.EventTypeWarning,
				reasonFeatureGateDisabled,
				msg,
			)
		}

		if feature == featurePartitionable {
			for i := range gpus {
				s.recorder.WithLogging(log.With("featureGate", feature)).Event(
					&gpus[i],
					corev1.EventTypeWarning,
					reasonExclusiveFallback,
					"publishing exclusive Physical offers only (no MIG profiles)",
				)
			}
		}
	}
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
