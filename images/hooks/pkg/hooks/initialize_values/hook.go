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

package initialize_values

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
}, handleInitializeValues)

func handleInitializeValues(_ context.Context, input *pkg.HookInput) error {
	for _, path := range []string{
		settings.ConfigRoot,
		settings.ConfigRoot + ".managedNodes",
		settings.ConfigRoot + ".deviceApproval",
		settings.ConfigRoot + ".scheduling",
		settings.ConfigRoot + ".inventory",
		settings.ConfigRoot + ".runtime",
		settings.ConfigRoot + ".monitoring",
		settings.ConfigRoot + ".images",
		settings.InternalRootPath,
		settings.InternalControllerPath,
		settings.InternalControllerPath + ".config",
		settings.InternalCertificatesPath,
	} {
		ensureMap(input, path)
	}
	return nil
}

func ensureMap(input *pkg.HookInput, path string) {
	val := input.Values.Get(path)
	if !val.Exists() || val.Type == gjson.Null {
		input.Values.Set(path, map[string]any{})
	}
}
