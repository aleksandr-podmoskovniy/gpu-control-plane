// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package validators

import (
	"fmt"
	"strings"

	"k8s.io/utils/ptr"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func Scheduling() SpecValidator {
	return func(spec *v1alpha1.GPUPoolSpec) error {
		switch spec.Scheduling.Strategy {
		case "", v1alpha1.GPUPoolSchedulingBinPack, v1alpha1.GPUPoolSchedulingSpread:
		default:
			return fmt.Errorf("unsupported scheduling.strategy %q", spec.Scheduling.Strategy)
		}
		if spec.Scheduling.Strategy == v1alpha1.GPUPoolSchedulingSpread && spec.Scheduling.TopologyKey == "" {
			return fmt.Errorf("scheduling.topologyKey is required when strategy=Spread")
		}
		if spec.Scheduling.TaintsEnabled == nil {
			spec.Scheduling.TaintsEnabled = ptr.To(true)
		}

		for i, t := range spec.Scheduling.Taints {
			if strings.TrimSpace(t.Key) == "" {
				return fmt.Errorf("taints[%d].key must be set", i)
			}
			spec.Scheduling.Taints[i].Key = strings.TrimSpace(t.Key)
			spec.Scheduling.Taints[i].Value = strings.TrimSpace(t.Value)
			spec.Scheduling.Taints[i].Effect = strings.TrimSpace(t.Effect)
		}
		return nil
	}
}
