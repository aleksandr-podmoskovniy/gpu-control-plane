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

package hostinfo

import (
	"bufio"
	"os"
	"regexp"
	"strings"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
)

func discoverOS(osReleasePath string) *gpuv1alpha1.OSInfo {
	path := osReleasePath
	if path == "" {
		path = defaultHostOSRelease
	}
	if _, err := os.Stat(path); err != nil {
		path = defaultOSRelease
	}

	release, err := parseOSRelease(path)
	if err != nil {
		return nil
	}

	return &gpuv1alpha1.OSInfo{
		ID:      release["ID"],
		Version: release["VERSION_ID"],
		Name:    release["NAME"],
	}
}

func parseOSRelease(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	re := regexp.MustCompile(`^(?P<key>\w+)=(?P<value>.+)`)
	release := map[string]string{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if m := re.FindStringSubmatch(line); m != nil {
			release[m[1]] = strings.Trim(m[2], `"'`)
		}
	}

	return release, nil
}
