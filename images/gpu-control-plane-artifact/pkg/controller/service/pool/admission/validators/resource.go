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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func Resource() SpecValidator {
	return func(spec *v1alpha1.GPUPoolSpec) error {
		if spec.Resource.Unit == "" {
			return fmt.Errorf("resource.unit must be set")
		}
		switch spec.Resource.Unit {
		case "Card":
			if spec.Resource.MIGProfile != "" {
				return fmt.Errorf("resource.migProfile is not allowed when unit=Card")
			}
		case "MIG":
			if spec.Resource.MIGProfile == "" {
				return fmt.Errorf("resource.migProfile is required when unit=MIG")
			}
			if !isValidMIGProfile(spec.Resource.MIGProfile) {
				return fmt.Errorf("resource.migProfile %q has invalid format", spec.Resource.MIGProfile)
			}
		default:
			return fmt.Errorf("unsupported resource.unit %q", spec.Resource.Unit)
		}

		if spec.Resource.SlicesPerUnit < 1 {
			return fmt.Errorf("resource.slicesPerUnit must be >= 1")
		}
		if spec.Resource.SlicesPerUnit > 64 {
			return fmt.Errorf("resource.slicesPerUnit must be <= 64")
		}

		if spec.Backend == "DRA" {
			if spec.Resource.Unit != "Card" {
				return fmt.Errorf("backend=DRA currently supports only unit=Card")
			}
			if spec.Resource.SlicesPerUnit > 1 {
				return fmt.Errorf("backend=DRA does not support slicesPerUnit>1")
			}
		}
		return nil
	}
}
