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
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func parseNVCapDeviceInfo(path string) (*nvCapDeviceInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	major, err := getDeviceMajor(nvidiaCapsDeviceName)
	if err != nil {
		return nil, fmt.Errorf("get %s major: %w", nvidiaCapsDeviceName, err)
	}

	info := &nvCapDeviceInfo{Major: major}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "DeviceFileMinor":
			if parsed, err := strconv.Atoi(value); err == nil {
				info.Minor = parsed
			}
		case "DeviceFileMode":
			if parsed, err := strconv.Atoi(value); err == nil {
				info.Mode = os.FileMode(parsed)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	info.Path = fmt.Sprintf("%s/nvidia-cap%d", devNvidiaCapsPath, info.Minor)
	return info, nil
}

func getDeviceMajor(name string) (int, error) {
	data, err := os.ReadFile(procDevicesPath)
	if err != nil {
		return 0, err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	inChar := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "Character devices:" {
			inChar = true
			continue
		}
		if line == "Block devices:" {
			break
		}
		if !inChar || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[1] == name {
			major, err := strconv.Atoi(fields[0])
			if err != nil {
				return 0, fmt.Errorf("parse major %q: %w", fields[0], err)
			}
			return major, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("device %q not found in %s", name, procDevicesPath)
}
