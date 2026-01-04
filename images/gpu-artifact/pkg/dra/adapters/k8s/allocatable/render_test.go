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

package allocatable

import (
	"testing"

	resourceapi "k8s.io/api/resource/v1"

	domain "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func TestRenderGPUDevice(t *testing.T) {
	vendor := "nvidia"
	ccMajor := int64(8)
	attrs := map[string]domain.AttributeValue{
		domain.AttrVendor:  {String: &vendor},
		domain.AttrCCMajor: {Int: &ccMajor},
	}
	capacity := map[string]domain.CapacityValue{
		domain.CapMemory: {Value: 2048, Unit: domain.CapacityUnitMiB},
		domain.CapSharePercent: {
			Value: 50,
			Unit:  domain.CapacityUnitPercent,
			Policy: &domain.CapacityPolicy{
				Default: 100,
				Min:     1,
				Max:     100,
				Step:    1,
				Unit:    domain.CapacityUnitPercent,
			},
		},
	}
	consumes := []domain.CounterConsumption{
		{
			CounterSet: "pgpu-0",
			Counters: map[string]domain.CounterValue{
				"memory": {Value: 1024, Unit: domain.CounterUnitMiB},
			},
		},
	}

	devices := RenderDevices([]domain.Device{
		domain.NewGPUDevice("gpu0", "GPU-1", attrs, capacity, consumes, true),
	})
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	device := devices[0]
	if device.Name != "gpu0" {
		t.Fatalf("expected name gpu0, got %q", device.Name)
	}
	if device.AllowMultipleAllocations == nil || !*device.AllowMultipleAllocations {
		t.Fatalf("expected AllowMultipleAllocations true")
	}

	if attr := device.Attributes[resourceapi.QualifiedName(domain.AttrVendor)]; attr.StringValue == nil || *attr.StringValue != "nvidia" {
		t.Fatalf("expected vendor attribute")
	}
	if attr := device.Attributes[resourceapi.QualifiedName(domain.AttrCCMajor)]; attr.IntValue == nil || *attr.IntValue != 8 {
		t.Fatalf("expected cc.major attribute")
	}

	mem := device.Capacity[resourceapi.QualifiedName(domain.CapMemory)]
	if mem.Value.Value() != 2048*1024*1024 {
		t.Fatalf("unexpected memory value: %d", mem.Value.Value())
	}
	share := device.Capacity[resourceapi.QualifiedName(domain.CapSharePercent)]
	if share.RequestPolicy == nil || share.RequestPolicy.ValidRange == nil {
		t.Fatalf("expected sharePercent request policy")
	}
	if share.RequestPolicy.ValidRange.Min.Value() != 1 {
		t.Fatalf("unexpected sharePercent min: %d", share.RequestPolicy.ValidRange.Min.Value())
	}

	if len(device.ConsumesCounters) != 1 {
		t.Fatalf("expected consumes counters")
	}
	counter, ok := device.ConsumesCounters[0].Counters["memory"]
	if !ok {
		t.Fatalf("expected memory counter")
	}
	if counter.Value.Value() != 1024*1024*1024 {
		t.Fatalf("unexpected consumes memory")
	}
}

func TestRenderMIGDeviceDefaults(t *testing.T) {
	devices := RenderDevices([]domain.Device{
		domain.NewMIGDevice("mig0", "", nil, nil, nil),
	})
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	device := devices[0]
	if device.AllowMultipleAllocations != nil {
		t.Fatalf("expected AllowMultipleAllocations to be nil")
	}
	if len(device.Attributes) != 0 || len(device.Capacity) != 0 {
		t.Fatalf("expected empty attributes and capacity")
	}
}

func TestRenderDevicesEmpty(t *testing.T) {
	if RenderDevices(nil) != nil {
		t.Fatalf("expected nil devices")
	}
}
