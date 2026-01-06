// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package allocator

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func TestAllocateExactCount(t *testing.T) {
	t.Parallel()

	svc := NewService()
	alloc, err := svc.Allocate(context.Background(), Input{
		Requests: []Request{{
			Name:      "gpu",
			Count:     1,
			Selectors: []Selector{allowAllSelector{}},
		}},
		Candidates: []CandidateDevice{{
			Key:      DeviceKey{Driver: DefaultDriverName, Pool: "pool-a", Device: "dev-1"},
			Driver:   DefaultDriverName,
			Pool:     "pool-a",
			NodeName: "node-1",
			Spec: allocatable.DeviceSpec{
				Name: "dev-1",
			},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alloc == nil {
		t.Fatalf("expected allocation result")
	}
	if len(alloc.Devices) != 1 {
		t.Fatalf("expected 1 device result, got %d", len(alloc.Devices))
	}
	got := alloc.Devices[0]
	if got.Driver != DefaultDriverName || got.Pool != "pool-a" || got.Device != "dev-1" {
		t.Fatalf("unexpected allocation result: %#v", got)
	}
	if alloc.NodeName != "node-1" {
		t.Fatalf("expected node-1, got %q", alloc.NodeName)
	}
}

func TestAllocateSkipsWhenNoMatch(t *testing.T) {
	t.Parallel()

	svc := NewService()
	alloc, err := svc.Allocate(context.Background(), Input{
		Requests: []Request{{
			Name:      "gpu",
			Count:     1,
			Selectors: []Selector{rejectAllSelector{}},
		}},
		Candidates: []CandidateDevice{{
			Key:      DeviceKey{Driver: DefaultDriverName, Pool: "pool-a", Device: "dev-1"},
			Driver:   DefaultDriverName,
			Pool:     "pool-a",
			NodeName: "node-1",
			Spec: allocatable.DeviceSpec{
				Name: "dev-1",
			},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alloc != nil {
		t.Fatalf("expected no allocation when selector rejects devices")
	}
}

func TestAllocateSharePercent(t *testing.T) {
	t.Parallel()

	svc := NewService()
	share := resource.MustParse("50")

	alloc, err := svc.Allocate(context.Background(), Input{
		Requests: []Request{{
			Name:      "gpu",
			Count:     2,
			Selectors: []Selector{allowAllSelector{}},
			Capacity: &CapacityRequirements{
				Requests: map[string]resource.Quantity{
					"sharePercent": share,
				},
			},
		}},
		Candidates: []CandidateDevice{{
			Key:      DeviceKey{Driver: DefaultDriverName, Pool: "pool-a", Device: "dev-1"},
			Driver:   DefaultDriverName,
			Pool:     "pool-a",
			NodeName: "node-1",
			Spec: allocatable.DeviceSpec{
				Name: "dev-1",
				Capacity: map[string]allocatable.CapacityValue{
					"sharePercent": {
						Value: 100,
						Unit:  allocatable.CapacityUnitPercent,
						Policy: &allocatable.CapacityPolicy{
							Default: 100,
							Min:     1,
							Max:     100,
							Step:    1,
							Unit:    allocatable.CapacityUnitPercent,
						},
					},
				},
				AllowMultipleAllocations: true,
			},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alloc == nil || len(alloc.Devices) != 2 {
		t.Fatalf("expected 2 allocations, got %#v", alloc)
	}
	for _, dev := range alloc.Devices {
		got, ok := dev.ConsumedCapacity["sharePercent"]
		if !ok {
			t.Fatalf("expected consumed capacity for sharePercent")
		}
		if got.Cmp(share) != 0 {
			t.Fatalf("unexpected sharePercent: %s", got.String())
		}
	}
}

func TestAllocateRespectsSharedCounters(t *testing.T) {
	t.Parallel()

	svc := NewService()
	counterSet := allocatable.CounterSet{
		Name: "pgpu-1",
		Counters: map[string]allocatable.CounterValue{
			"memory":         {Value: 80, Unit: allocatable.CounterUnitMiB},
			"memory-slice-0": {Value: 1, Unit: allocatable.CounterUnitCount},
			"memory-slice-1": {Value: 1, Unit: allocatable.CounterUnitCount},
		},
	}

	alloc, err := svc.Allocate(context.Background(), Input{
		Requests: []Request{{
			Name:      "gpu",
			Count:     2,
			Selectors: []Selector{allowAllSelector{}},
		}},
		Candidates: []CandidateDevice{
			{
				Key:      DeviceKey{Driver: DefaultDriverName, Pool: "pool-a", Device: "mig-1"},
				Driver:   DefaultDriverName,
				Pool:     "pool-a",
				NodeName: "node-1",
				Spec: allocatable.DeviceSpec{
					Name: "mig-1",
					Consumes: []allocatable.CounterConsumption{{
						CounterSet: counterSet.Name,
						Counters: map[string]allocatable.CounterValue{
							"memory":         {Value: 50, Unit: allocatable.CounterUnitMiB},
							"memory-slice-0": {Value: 1, Unit: allocatable.CounterUnitCount},
						},
					}},
				},
			},
			{
				Key:      DeviceKey{Driver: DefaultDriverName, Pool: "pool-a", Device: "mig-2"},
				Driver:   DefaultDriverName,
				Pool:     "pool-a",
				NodeName: "node-1",
				Spec: allocatable.DeviceSpec{
					Name: "mig-2",
					Consumes: []allocatable.CounterConsumption{{
						CounterSet: counterSet.Name,
						Counters: map[string]allocatable.CounterValue{
							"memory":         {Value: 50, Unit: allocatable.CounterUnitMiB},
							"memory-slice-1": {Value: 1, Unit: allocatable.CounterUnitCount},
						},
					}},
				},
			},
		},
		CounterSets: CounterSetInventory{
			"node-1": {
				counterSet.Name: counterSet,
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alloc != nil {
		t.Fatalf("expected allocation to fail due to shared counters, got %#v", alloc)
	}
}

func TestAllocateSkipsMixedTypesSameCounterSet(t *testing.T) {
	t.Parallel()

	svc := NewService()
	pci := "0000:02:00.0"
	counterSet := allocatable.CounterSetNameForPCI(pci)
	migType := string(gpuv1alpha1.DeviceTypeMIG)
	physType := string(gpuv1alpha1.DeviceTypePhysical)

	migKey := DeviceKey{Driver: DefaultDriverName, Pool: "pool-a", Device: "mig-1"}
	physKey := DeviceKey{Driver: DefaultDriverName, Pool: "pool-a", Device: "gpu-1"}

	alloc, err := svc.Allocate(context.Background(), Input{
		Requests: []Request{{
			Name:      "gpu",
			Count:     1,
			Selectors: []Selector{allowAllSelector{}},
		}},
		Candidates: []CandidateDevice{
			{
				Key:      migKey,
				Driver:   DefaultDriverName,
				Pool:     "pool-a",
				NodeName: "node-1",
				Spec: allocatable.DeviceSpec{
					Name: "mig-1",
					Attributes: map[string]allocatable.AttributeValue{
						allocatable.AttrDeviceType: {String: &migType},
						allocatable.AttrPCIAddress: {String: &pci},
					},
					Consumes: []allocatable.CounterConsumption{
						{CounterSet: counterSet},
					},
				},
			},
			{
				Key:      physKey,
				Driver:   DefaultDriverName,
				Pool:     "pool-a",
				NodeName: "node-1",
				Spec: allocatable.DeviceSpec{
					Name: "gpu-1",
					Attributes: map[string]allocatable.AttributeValue{
						allocatable.AttrDeviceType: {String: &physType},
						allocatable.AttrPCIAddress: {String: &pci},
					},
				},
			},
		},
		Allocated: map[DeviceKey]AllocatedDeviceInfo{
			migKey: {Exclusive: true},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alloc != nil {
		t.Fatalf("expected no allocation when MIG is already allocated on the same GPU")
	}
}

type allowAllSelector struct{}

func (allowAllSelector) Match(context.Context, string, allocatable.DeviceSpec) (bool, error) {
	return true, nil
}

type rejectAllSelector struct{}

func (rejectAllSelector) Match(context.Context, string, allocatable.DeviceSpec) (bool, error) {
	return false, nil
}
