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

package cdi

import (
	"context"
	"strings"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/dynamic-resource-allocation/resourceslice"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/cdi/nvcdi"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

// BaseSpecWriter writes the base CDI spec entries.
type BaseSpecWriter interface {
	WriteBase(ctx context.Context, devices []nvcdi.BaseDevice) error
}

// BaseSpecSyncer syncs base CDI specs from ResourceSlice data.
type BaseSpecSyncer struct {
	writer BaseSpecWriter
}

// NewBaseSpecSyncer creates a BaseSpecSyncer.
func NewBaseSpecSyncer(writer BaseSpecWriter) *BaseSpecSyncer {
	return &BaseSpecSyncer{writer: writer}
}

// Sync builds base devices from DriverResources and writes the base CDI spec.
func (s *BaseSpecSyncer) Sync(ctx context.Context, resources resourceslice.DriverResources) error {
	if s == nil || s.writer == nil {
		return nil
	}
	devices := baseDevices(resources)
	if len(devices) == 0 {
		return nil
	}
	return s.writer.WriteBase(ctx, devices)
}

func baseDevices(resources resourceslice.DriverResources) []nvcdi.BaseDevice {
	if len(resources.Pools) == 0 {
		return nil
	}

	seen := map[string]nvcdi.BaseDevice{}
	for _, pool := range resources.Pools {
		for _, slice := range pool.Slices {
			for _, device := range slice.Devices {
				if !isPhysicalDevice(device.Attributes) {
					continue
				}
				uuid := attrString(device.Attributes, allocatable.AttrGPUUUID)
				if uuid == "" {
					continue
				}
				name := strings.TrimSpace(device.Name)
				if name == "" {
					continue
				}
				seen[name] = nvcdi.BaseDevice{Name: name, UUID: uuid}
			}
		}
	}

	devices := make([]nvcdi.BaseDevice, 0, len(seen))
	for _, device := range seen {
		devices = append(devices, device)
	}
	return devices
}

func isPhysicalDevice(attrs map[resourcev1.QualifiedName]resourcev1.DeviceAttribute) bool {
	deviceType := strings.ToLower(attrString(attrs, allocatable.AttrDeviceType))
	return deviceType == "physical"
}

func attrString(attrs map[resourcev1.QualifiedName]resourcev1.DeviceAttribute, key string) string {
	if len(attrs) == 0 {
		return ""
	}
	attr, ok := attrs[resourcev1.QualifiedName(key)]
	if !ok || attr.StringValue == nil {
		return ""
	}
	return strings.TrimSpace(*attr.StringValue)
}
