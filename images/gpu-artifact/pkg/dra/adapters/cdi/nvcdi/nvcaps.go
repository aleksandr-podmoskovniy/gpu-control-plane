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
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

const (
	procDevicesPath       = "/proc/devices"
	procNvCapsPath        = "/proc/driver/nvidia/capabilities"
	devNvidiaCapsPath     = "/dev/nvidia-caps"
	nvidiaCapsDeviceName  = "nvidia-caps"
)

type nvCapDeviceInfo struct {
	Major int
	Minor int
	Mode  os.FileMode
	Path  string
}

func (i *nvCapDeviceInfo) deviceNode() *cdispec.DeviceNode {
	if i == nil {
		return nil
	}
	mode := i.Mode
	return &cdispec.DeviceNode{
		Path:     i.Path,
		Type:     "c",
		FileMode: &mode,
		Major:    int64(i.Major),
		Minor:    int64(i.Minor),
	}
}

func (w *Writer) migDeviceNodes(migUUID string) ([]*cdispec.DeviceNode, error) {
	if w == nil || w.nvml == nil {
		return nil, errors.New("NVML is not configured")
	}
	if strings.TrimSpace(migUUID) == "" {
		return nil, errors.New("MIG UUID is required")
	}

	migDevice, ret := w.nvml.DeviceGetHandleByUUID(migUUID)
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("get MIG device by UUID %q: %s", migUUID, nvml.ErrorString(ret))
	}

	giID, ret := migDevice.GetGpuInstanceId()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("get GPU instance id for %q: %s", migUUID, nvml.ErrorString(ret))
	}
	ciID, ret := migDevice.GetComputeInstanceId()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("get compute instance id for %q: %s", migUUID, nvml.ErrorString(ret))
	}
	parent, ret := migDevice.GetDeviceHandleFromMigDeviceHandle()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("get parent GPU for %q: %s", migUUID, nvml.ErrorString(ret))
	}
	parentMinor, ret := parent.GetMinorNumber()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("get parent minor for %q: %s", migUUID, nvml.ErrorString(ret))
	}

	giPath := fmt.Sprintf("%s/gpu%d/mig/gi%d/access", procNvCapsPath, parentMinor, giID)
	ciPath := fmt.Sprintf("%s/gpu%d/mig/gi%d/ci%d/access", procNvCapsPath, parentMinor, giID, ciID)

	giInfo, err := parseNVCapDeviceInfo(giPath)
	if err != nil {
		return nil, fmt.Errorf("parse GI caps %q: %w", giPath, err)
	}
	ciInfo, err := parseNVCapDeviceInfo(ciPath)
	if err != nil {
		return nil, fmt.Errorf("parse CI caps %q: %w", ciPath, err)
	}

	return []*cdispec.DeviceNode{giInfo.deviceNode(), ciInfo.deviceNode()}, nil
}

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
