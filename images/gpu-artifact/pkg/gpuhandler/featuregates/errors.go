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
	"errors"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	dresourceslice "k8s.io/dynamic-resource-allocation/resourceslice"
)

func (s *Service) HandleError(ctx context.Context, err error, msg string) {
	var dropped *dresourceslice.DroppedFieldsError
	if errors.As(err, &dropped) {
		disabled := dropped.DisabledFeatures()
		if len(disabled) == 0 {
			if s.log != nil {
				s.log.Warn("DRA fields dropped without detected feature gate", "pool", dropped.PoolName, "sliceIndex", dropped.SliceIndex)
			}
			utilruntime.HandleErrorWithContext(ctx, err, msg)
			return
		}

		known, unknown := splitFeatures(disabled)
		if len(known) > 0 && s.builder != nil {
			if s.builder.DisableFeatures(known) && s.log != nil {
				s.log.Warn("DRA features disabled after apiserver dropped fields", "features", known, "pool", dropped.PoolName, "sliceIndex", dropped.SliceIndex)
				if s.notify != nil {
					s.notify()
				}
			}
		}

		s.recordFeatureGateEvents(ctx, dropped.PoolName, known)
		if len(unknown) > 0 && s.log != nil {
			s.log.Warn("DRA fields dropped for unsupported features", "features", unknown, "pool", dropped.PoolName, "sliceIndex", dropped.SliceIndex)
			utilruntime.HandleErrorWithContext(ctx, err, msg)
		}
		return
	}

	utilruntime.HandleErrorWithContext(ctx, err, msg)
}
