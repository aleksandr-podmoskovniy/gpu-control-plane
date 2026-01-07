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

package nvcdi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

func devRootFor(root string) string {
	if root == "" {
		return "/"
	}
	info, err := os.Stat(filepath.Join(root, "dev"))
	if err != nil {
		return "/"
	}
	if info.IsDir() {
		return root
	}
	return "/"
}

func nvmlLibraryOptions(driverRoot string) []nvml.LibraryOption {
	driverRoot = strings.TrimSpace(driverRoot)
	if driverRoot == "" {
		return nil
	}
	path, err := findNVMLLibrary(driverRoot)
	if err != nil {
		return nil
	}
	return []nvml.LibraryOption{nvml.WithLibraryPath(path)}
}

func findNVMLLibrary(root string) (string, error) {
	for _, dir := range nvmlLibrarySearchPaths {
		candidate := filepath.Join(root, dir, "libnvidia-ml.so.1")
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
	return "", fmt.Errorf("nvml library not found under %s", root)
}

var nvmlLibrarySearchPaths = []string{
	"/usr/lib64",
	"/usr/lib/x86_64-linux-gnu",
	"/usr/lib/aarch64-linux-gnu",
	"/lib64",
	"/lib/x86_64-linux-gnu",
	"/lib/aarch64-linux-gnu",
}
