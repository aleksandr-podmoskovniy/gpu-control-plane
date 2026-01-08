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
	"os"

	cdispec "tags.cncf.io/container-device-interface/specs-go"
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
