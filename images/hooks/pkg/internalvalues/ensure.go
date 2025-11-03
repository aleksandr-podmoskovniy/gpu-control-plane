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

package internalvalues

import (
	"github.com/tidwall/gjson"

	pkg "github.com/deckhouse/module-sdk/pkg"

	"hooks/pkg/settings"
)

var ensurePaths = []string{
	settings.ConfigRoot,
	settings.ConfigRoot + ".internal",
	settings.InternalModuleConfigPath,
	settings.InternalModuleValidationPath,
	settings.InternalModuleConditionsPath,
	settings.InternalBootstrapPath,
	settings.InternalControllerPath,
	settings.InternalControllerCertPath,
	settings.InternalRootCAPath,
	settings.InternalCustomCertificatePath,
}

// Ensure guarantees that all internal values paths required by module hooks exist as JSON objects.
func Ensure(input *pkg.HookInput) {
	for _, path := range ensurePaths {
		ensureMap(input, path)
	}
}

func ensureMap(input *pkg.HookInput, path string) {
	value := input.Values.Get(path)
	if !value.Exists() || value.Type != gjson.JSON || !value.IsObject() {
		input.Values.Set(path, map[string]any{})
	}
}
