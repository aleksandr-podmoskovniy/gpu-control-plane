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

package pci

import (
	"os"
	"path/filepath"
	"strings"
)

func normalizeHexID(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(strings.ToLower(value), "0x")
	return value
}

func normalizeClassCode(raw string) string {
	value := normalizeHexID(raw)
	if len(value) < 4 {
		return ""
	}
	return value[:4]
}

func readTrim(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func readDriverName(devicePath string) string {
	link, err := os.Readlink(filepath.Join(devicePath, "driver"))
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}
