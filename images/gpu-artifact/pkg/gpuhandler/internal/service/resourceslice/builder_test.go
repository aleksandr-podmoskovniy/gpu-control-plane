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

package resourceslice

import (
	"context"
	"errors"
	"testing"

	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/dynamic-resource-allocation/resourceslice"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory"
)

func TestResourceSliceBuilderEmpty(t *testing.T) {
	builder := NewBuilder(nil)
	resources, err := builder.Build(context.Background(), "node-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pool, ok := resources.Pools["gpus/node-1"]
	if !ok {
		t.Fatalf("expected pool for gpus/node-1")
	}
	if len(pool.Slices) != 1 {
		t.Fatalf("expected 1 slice, got %d", len(pool.Slices))
	}
	if len(pool.Slices[0].Devices) != 0 {
		t.Fatalf("expected empty device slice")
	}
}

func TestResourceSliceBuilderPhysicalOnly(t *testing.T) {
	builder := NewBuilder(nil)
	pgpu := sampleGPU("0000:02:00.0", false)

	resources, err := builder.Build(context.Background(), "node-1", []gpuv1alpha1.PhysicalGPU{pgpu})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pool := resources.Pools["gpus/node-1"]
	if len(pool.Slices) != 1 {
		t.Fatalf("expected 1 slice, got %d", len(pool.Slices))
	}
	if len(pool.Slices[0].Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(pool.Slices[0].Devices))
	}

	device := pool.Slices[0].Devices[0]
	if device.AllowMultipleAllocations == nil || !*device.AllowMultipleAllocations {
		t.Fatalf("expected AllowMultipleAllocations true")
	}
	if _, ok := device.Capacity[resourceapi.QualifiedName(allocatable.CapSharePercent)]; !ok {
		t.Fatalf("expected sharePercent capacity")
	}
	if _, ok := device.Capacity[resourceapi.QualifiedName(allocatable.CapMemory)]; !ok {
		t.Fatalf("expected memory capacity")
	}
}

func TestResourceSliceBuilderMigSupported(t *testing.T) {
	pgpu := sampleGPU("0000:02:00.0", true)
	placements := map[string]map[int32][]inventory.MigPlacement{
		"0000:02:00.0": {
			5: {
				{Start: 0, Size: 2},
				{Start: 2, Size: 2},
			},
		},
	}
	builder := NewBuilder(&fakePlacementReader{placements: placements})

	resources, err := builder.Build(context.Background(), "node-1", []gpuv1alpha1.PhysicalGPU{pgpu})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pool := resources.Pools["gpus/node-1"]
	if len(pool.Slices) != 2 {
		t.Fatalf("expected 2 slices, got %d", len(pool.Slices))
	}

	counterSlice, deviceSlice := splitSlices(pool.Slices)
	if counterSlice == nil {
		t.Fatalf("expected counter slice")
	}
	if deviceSlice == nil {
		t.Fatalf("expected device slice")
	}

	if len(counterSlice.SharedCounters) != 1 {
		t.Fatalf("expected 1 counter set, got %d", len(counterSlice.SharedCounters))
	}
	counterSet := counterSlice.SharedCounters[0]
	if _, ok := counterSet.Counters["memory"]; !ok {
		t.Fatalf("expected memory counter")
	}
	if _, ok := counterSet.Counters["memorySlice0"]; !ok {
		t.Fatalf("expected memorySlice0 counter")
	}

	migCount := 0
	for _, dev := range deviceSlice.Devices {
		if attr, ok := dev.Attributes[resourceapi.QualifiedName(allocatable.AttrDeviceType)]; ok && attr.StringValue != nil && *attr.StringValue == string(gpuv1alpha1.DeviceTypeMIG) {
			migCount++
			if len(dev.ConsumesCounters) == 0 {
				t.Fatalf("expected consumes counters on MIG device")
			}
		}
	}
	if migCount != 2 {
		t.Fatalf("expected 2 MIG devices, got %d", migCount)
	}
}

func TestResourceSliceBuilderMigPlacementError(t *testing.T) {
	pgpu := sampleGPU("0000:02:00.0", true)
	builder := NewBuilder(&fakePlacementReader{err: errors.New("nvml error")})

	resources, err := builder.Build(context.Background(), "node-1", []gpuv1alpha1.PhysicalGPU{pgpu})
	if err == nil {
		t.Fatalf("expected error")
	}
	pool := resources.Pools["gpus/node-1"]
	if len(pool.Slices) == 0 {
		t.Fatalf("expected slices to be published")
	}
}

type fakePlacementReader struct {
	placements map[string]map[int32][]inventory.MigPlacement
	err        error
}

func (f *fakePlacementReader) Open() (inventory.MigPlacementSession, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &fakePlacementSession{placements: f.placements}, nil
}

type fakePlacementSession struct {
	placements map[string]map[int32][]inventory.MigPlacement
}

func (s *fakePlacementSession) Close() {}

func (s *fakePlacementSession) ReadPlacements(pciAddress string, _ []int32) (map[int32][]inventory.MigPlacement, error) {
	if s.placements == nil {
		return map[int32][]inventory.MigPlacement{}, nil
	}
	if byProfile, ok := s.placements[pciAddress]; ok {
		return byProfile, nil
	}
	return map[int32][]inventory.MigPlacement{}, nil
}

func sampleGPU(pciAddress string, migSupported bool) gpuv1alpha1.PhysicalGPU {
	mem := int64(24576)
	migSupportedPtr := &migSupported
	gpu := gpuv1alpha1.PhysicalGPU{
		ObjectMeta: metav1.ObjectMeta{Name: "pgpu-1"},
		Status: gpuv1alpha1.PhysicalGPUStatus{
			PCIInfo: &gpuv1alpha1.PCIInfo{
				Address: pciAddress,
				Vendor:  &gpuv1alpha1.PCIVendorInfo{Name: "NVIDIA Corporation"},
				Device:  &gpuv1alpha1.PCIDeviceInfo{Name: "GA100GL [A30 PCIe]"},
			},
			Capabilities: &gpuv1alpha1.GPUCapabilities{
				ProductName: "NVIDIA A30",
				MemoryMiB:   &mem,
				Vendor:      gpuv1alpha1.VendorNvidia,
				Nvidia: &gpuv1alpha1.NvidiaCapabilities{
					ComputeCap:   "8.0",
					MIGSupported: migSupportedPtr,
				},
			},
			CurrentState: &gpuv1alpha1.GPUCurrentState{
				DriverType: gpuv1alpha1.DriverTypeNvidia,
				Nvidia: &gpuv1alpha1.NvidiaCurrentState{
					GPUUUID:       "GPU-123",
					DriverVersion: "580.76.05",
				},
			},
		},
	}

	if migSupported {
		gpu.Status.Capabilities.Nvidia.MIG = &gpuv1alpha1.NvidiaMIGCapabilities{
			TotalSlices: 4,
			Profiles: []gpuv1alpha1.NvidiaMIGProfile{
				{
					ProfileID:    5,
					Name:         "2g.12gb",
					MemoryMiB:    12032,
					SliceCount:   2,
					MaxInstances: 2,
				},
			},
		}
	}
	return gpu
}

func splitSlices(slices []resourceslice.Slice) (*resourceslice.Slice, *resourceslice.Slice) {
	var counterSlice *resourceslice.Slice
	var deviceSlice *resourceslice.Slice
	for i := range slices {
		if len(slices[i].SharedCounters) > 0 {
			counterSlice = &slices[i]
			continue
		}
		deviceSlice = &slices[i]
	}
	return counterSlice, deviceSlice
}
