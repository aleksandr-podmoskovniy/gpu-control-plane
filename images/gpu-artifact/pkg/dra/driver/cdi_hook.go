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

package driver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultNvidiaCDIHookSource = "/usr/bin/nvidia-cdi-hook"
const nvidiaCDIHookFilename = "nvidia-cdi-hook"

// ResolveNvidiaCDIHookPath ensures nvidia-cdi-hook is available and returns its path.
func ResolveNvidiaCDIHookPath(explicitPath, pluginPath string) (string, error) {
	if strings.TrimSpace(explicitPath) != "" {
		return explicitPath, nil
	}
	if strings.TrimSpace(pluginPath) == "" {
		return "", errors.New("plugin path is required to stage nvidia-cdi-hook")
	}

	targetPath := filepath.Join(pluginPath, nvidiaCDIHookFilename)
	if _, err := os.Stat(targetPath); err == nil {
		return targetPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat nvidia-cdi-hook target: %w", err)
	}

	input, err := os.ReadFile(defaultNvidiaCDIHookSource)
	if err != nil {
		return "", fmt.Errorf("read nvidia-cdi-hook: %w", err)
	}
	if err := os.WriteFile(targetPath, input, 0o755); err != nil {
		return "", fmt.Errorf("copy nvidia-cdi-hook: %w", err)
	}

	return targetPath, nil
}
