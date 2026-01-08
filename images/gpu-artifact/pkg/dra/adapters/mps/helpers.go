//go:build linux
// +build linux

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

package mps

import (
	"os"
	"path/filepath"
	"strings"
)

func resolveBinary(root string, relDirs []string, name string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	for _, rel := range relDirs {
		path := filepath.Join(root, rel, name)
		if isRegularFile(path) {
			return path
		}
	}
	return ""
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func ensureDir(path string) error {
	if path == "" {
		return nil
	}
	return os.MkdirAll(path, 0o755)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func withLDLibraryPath(env []string, driverRoot string) []string {
	driverRoot = strings.TrimSpace(driverRoot)
	if driverRoot == "" {
		return env
	}
	paths := []string{
		filepath.Join(driverRoot, "usr/lib64"),
		filepath.Join(driverRoot, "usr/lib/x86_64-linux-gnu"),
		filepath.Join(driverRoot, "usr/lib/aarch64-linux-gnu"),
		filepath.Join(driverRoot, "lib64"),
		filepath.Join(driverRoot, "lib/x86_64-linux-gnu"),
		filepath.Join(driverRoot, "lib/aarch64-linux-gnu"),
	}
	for i := len(paths) - 1; i >= 0; i-- {
		if !isDir(paths[i]) {
			paths = append(paths[:i], paths[i+1:]...)
		}
	}
	if len(paths) == 0 {
		return env
	}

	const key = "LD_LIBRARY_PATH"
	value := strings.Join(paths, ":")
	for i, entry := range env {
		if strings.HasPrefix(entry, key+"=") {
			current := strings.TrimPrefix(entry, key+"=")
			if current == "" {
				env[i] = key + "=" + value
			} else {
				env[i] = key + "=" + value + ":" + current
			}
			return env
		}
	}
	return append(env, key+"="+value)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
