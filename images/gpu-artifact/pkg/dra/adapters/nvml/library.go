//go:build linux && cgo && nvml
// +build linux,cgo,nvml

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

package nvml

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	nvmlapi "github.com/NVIDIA/go-nvml/pkg/nvml"
)

const nvmlLibraryName = "libnvidia-ml.so.1"

var nvmlLibrarySearchPaths = []string{
	"/usr/lib64",
	"/usr/lib/x86_64-linux-gnu",
	"/usr/lib/aarch64-linux-gnu",
	"/lib64",
	"/lib/x86_64-linux-gnu",
	"/lib/aarch64-linux-gnu",
}

func normalizePCIBusID(busID string) string {
	busID = strings.TrimSpace(busID)
	parts := strings.Split(busID, ":")
	if len(parts) != 3 {
		return busID
	}
	if len(parts[0]) < 8 {
		if value, err := strconv.ParseUint(parts[0], 16, 32); err == nil {
			parts[0] = fmt.Sprintf("%08x", value)
		}
	}
	return strings.Join(parts, ":")
}

func nvmlLibraryOptions() []nvmlapi.LibraryOption {
	driverRoot := strings.TrimSpace(os.Getenv("NVIDIA_DRIVER_ROOT"))
	if driverRoot == "" {
		return nil
	}
	path, err := findNVMLLibrary(driverRoot)
	if err == nil {
		return []nvmlapi.LibraryOption{nvmlapi.WithLibraryPath(path)}
	}
	return nil
}

func findNVMLLibrary(root string) (string, error) {
	for _, dir := range nvmlLibrarySearchPaths {
		candidate := filepath.Join(root, dir, nvmlLibraryName)
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || info.IsDir() {
			continue
		}
		return resolved, nil
	}
	return "", fmt.Errorf("nvml library %q not found under %s", nvmlLibraryName, root)
}
