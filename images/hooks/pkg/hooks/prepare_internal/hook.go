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

package prepare_internal

import (
	"context"

	"github.com/tidwall/gjson"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"

	"hooks/pkg/settings"
)

var _ = registry.RegisterFunc(&pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 0},
	Queue:        settings.ModuleQueue,
}, handlePrepareInternal)

func handlePrepareInternal(_ context.Context, input *pkg.HookInput) error {
	for _, path := range []string{
		settings.ConfigRoot,
		settings.ConfigRoot + ".internal",
		settings.InternalRootCAPath,
		settings.InternalControllerPath,
		settings.InternalControllerCertPath,
		settings.InternalCustomCertificatePath,
	} {
		ensureMap(input, path)
	}

	return nil
}

func ensureMap(input *pkg.HookInput, path string) {
	value := input.Values.Get(path)
	if !value.Exists() || value.Type != gjson.JSON || !value.IsObject() {
		input.Values.Set(path, map[string]any{})
	}
}
