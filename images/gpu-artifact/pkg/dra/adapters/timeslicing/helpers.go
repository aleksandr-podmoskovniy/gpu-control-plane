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

package timeslicing

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

func withLDPreload(env []string, libPath string) []string {
	if libPath == "" {
		return env
	}
	const key = "LD_PRELOAD"
	prefix := key + "=" + libPath
	for i, entry := range env {
		if strings.HasPrefix(entry, key+"=") {
			current := strings.TrimPrefix(entry, key+"=")
			if current == "" {
				env[i] = prefix
				return env
			}
			env[i] = key + "=" + libPath + ":" + current
			return env
		}
	}
	return append(env, prefix)
}
