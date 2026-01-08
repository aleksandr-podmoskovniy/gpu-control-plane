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
	"errors"
	"fmt"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

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
