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

import "testing"

func TestGPUDeviceSpec(t *testing.T) {
	vendor := "nvidia"
	ccMajor := int64(8)
	memCounter := int64(1024)
	attrs := map[string]AttributeValue{
		AttrVendor:  {String: &vendor},
		AttrCCMajor: {Int: &ccMajor},
	}
	capacity := map[string]CapacityValue{
		CapMemory: {Value: 2048, Unit: CapacityUnitMiB},
		CapSharePercent: {
			Value: 50,
			Unit:  CapacityUnitPercent,
			Policy: &CapacityPolicy{
				Default: 100,
				Min:     1,
				Max:     100,
				Step:    1,
				Unit:    CapacityUnitPercent,
			},
		},
	}
	consumes := []CounterConsumption{
		{
			CounterSet: "pgpu-0",
			Counters: map[string]CounterValue{
				"memory": {Value: memCounter, Unit: CounterUnitMiB},
			},
		},
	}

	spec := NewGPUDevice("gpu0", "GPU-1", attrs, capacity, consumes, true).Spec()
	if spec.Name != "gpu0" {
		t.Fatalf("expected name gpu0, got %q", spec.Name)
	}
	if !spec.AllowMultipleAllocations {
		t.Fatalf("expected AllowMultipleAllocations true")
	}

	if attr := spec.Attributes[AttrVendor]; attr.String == nil || *attr.String != "nvidia" {
		t.Fatalf("expected vendor attribute")
	}
	if attr := spec.Attributes[AttrCCMajor]; attr.Int == nil || *attr.Int != 8 {
		t.Fatalf("expected cc.major attribute")
	}

	mem := spec.Capacity[CapMemory]
	if mem.Value != 2048 {
		t.Fatalf("unexpected memory value: %d", mem.Value)
	}
	share := spec.Capacity[CapSharePercent]
	if share.Policy == nil || share.Policy.Min != 1 {
		t.Fatalf("expected sharePercent policy with min=1")
	}

	if len(spec.Consumes) != 1 {
		t.Fatalf("expected consumes counters")
	}
	counter, ok := spec.Consumes[0].Counters["memory"]
	if !ok {
		t.Fatalf("expected memory counter")
	}
	if counter.Value != 1024 {
		t.Fatalf("unexpected consumes memory")
	}
	if len(spec.BindingConditions) != 1 || spec.BindingConditions[0] != DeviceConditionReady {
		t.Fatalf("expected binding condition %q", DeviceConditionReady)
	}
	if len(spec.BindingFailureConditions) != 1 || spec.BindingFailureConditions[0] != DeviceConditionReady {
		t.Fatalf("expected binding failure condition %q", DeviceConditionReady)
	}
}

func TestMIGDeviceSpecDefaults(t *testing.T) {
	spec := NewMIGDevice("mig0", "", nil, nil, nil).Spec()
	if spec.AllowMultipleAllocations {
		t.Fatalf("expected AllowMultipleAllocations false")
	}
	if len(spec.Attributes) != 0 || len(spec.Capacity) != 0 {
		t.Fatalf("expected empty attributes and capacity")
	}
	if len(spec.BindingConditions) != 1 || spec.BindingConditions[0] != DeviceConditionReady {
		t.Fatalf("expected binding condition %q", DeviceConditionReady)
	}
	if len(spec.BindingFailureConditions) != 1 || spec.BindingFailureConditions[0] != DeviceConditionReady {
		t.Fatalf("expected binding failure condition %q", DeviceConditionReady)
	}
}
