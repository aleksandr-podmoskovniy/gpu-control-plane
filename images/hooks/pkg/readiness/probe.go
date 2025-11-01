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

package readiness

import (
	"context"
	"fmt"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"

	"hooks/pkg/settings"
)

var ReadinessConfig = app.ReadinessConfig{
	ProbeFunc: checkModuleReadiness,
}

func checkModuleReadiness(_ context.Context, input *pkg.HookInput) error {
	validation := input.Values.Get(settings.InternalModuleValidationPath)
	if validation.Exists() {
		if validation.IsObject() {
			if errField := validation.Get("error"); errField.Exists() {
				return fmt.Errorf("%s", errField.String())
			}
			return fmt.Errorf("module is not ready yet")
		}
		return fmt.Errorf("module validation state is malformed")
	}

	return nil
}
