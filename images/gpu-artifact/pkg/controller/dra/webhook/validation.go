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

package webhook

import (
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"

	configapi "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/configapi"
)

func validateDeviceConfigs(configs []resourcev1.DeviceClaimConfiguration, specPath, driverName string) []error {
	var errs []error
	for idx, cfg := range configs {
		opaque := cfg.Opaque
		if opaque == nil || opaque.Driver != driverName {
			continue
		}

		fieldPath := fmt.Sprintf("%s.devices.config[%d].opaque.parameters", specPath, idx)
		decoded, err := runtime.Decode(configapi.StrictDecoder, opaque.Parameters.Raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("error decoding object at %s: %w", fieldPath, err))
			continue
		}

		config, ok := decoded.(configapi.Interface)
		if !ok {
			errs = append(errs, fmt.Errorf("expected a recognized configuration type at %s but got: %T", fieldPath, decoded))
			continue
		}

		if err := config.Normalize(); err != nil {
			errs = append(errs, fmt.Errorf("error normalizing config at %s: %w", fieldPath, err))
			continue
		}
		if err := config.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("object at %s is invalid: %w", fieldPath, err))
		}
	}
	return errs
}
