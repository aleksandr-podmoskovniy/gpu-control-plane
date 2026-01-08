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

package pciids

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Load parses a pci.ids file.
func Load(path string) (*Resolver, error) {
	if path == "" {
		return nil, fmt.Errorf("pci.ids path is empty")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	res := &Resolver{
		vendors:   map[string]string{},
		devices:   map[string]map[string]string{},
		classBase: map[string]string{},
		classSub:  map[string]map[string]string{},
	}

	scanner := bufio.NewScanner(file)
	if err := parsePCIIDs(scanner, res); err != nil {
		return nil, err
	}

	return res, nil
}

// LoadFirst tries to load the first existing pci.ids from paths.
func LoadFirst(paths []string) (*Resolver, string, error) {
	var lastErr error
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		res, err := Load(path)
		if err == nil {
			return res, path, nil
		}
		if os.IsNotExist(err) {
			lastErr = err
			continue
		}
		return nil, path, err
	}
	if lastErr != nil {
		return nil, "", nil
	}
	return nil, "", nil
}
