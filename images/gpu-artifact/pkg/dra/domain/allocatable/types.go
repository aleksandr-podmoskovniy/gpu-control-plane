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

// AttributeValue represents a typed device attribute value.
type AttributeValue struct {
	String *string
	Int    *int64
	Bool   *bool
}

// DeviceSpec describes a device offer without k8s types.
type DeviceSpec struct {
	Name                     string
	Attributes               map[string]AttributeValue
	Capacity                 map[string]CapacityValue
	Consumes                 []CounterConsumption
	AllowMultipleAllocations bool
	BindingConditions        []string
	BindingFailureConditions []string
}

// CapacityValue represents a typed device capacity value.
type CapacityValue struct {
	Value  int64
	Unit   CapacityUnit
	Policy *CapacityPolicy
}

// CapacityPolicy controls how a capacity can be requested.
type CapacityPolicy struct {
	Default int64
	Min     int64
	Max     int64
	Step    int64
	Unit    CapacityUnit
}

// CounterSet represents shared counters for partitionable devices.
type CounterSet struct {
	Name     string
	Counters map[string]CounterValue
}

// CounterValue represents a typed counter value.
type CounterValue struct {
	Value int64
	Unit  CounterUnit
}

// CounterConsumption represents device consumption from a counter set.
type CounterConsumption struct {
	CounterSet string
	Counters   map[string]CounterValue
}

// Inventory groups devices and shared counters.
type Inventory struct {
	Devices     DeviceList
	CounterSets []CounterSet
}

// CapacityUnit defines units for capacity values.
type CapacityUnit string

const (
	CapacityUnitMiB     CapacityUnit = "MiB"
	CapacityUnitPercent CapacityUnit = "Percent"
	CapacityUnitCount   CapacityUnit = "Count"
)

// CounterUnit defines units for counter values.
type CounterUnit string

const (
	CounterUnitMiB   CounterUnit = "MiB"
	CounterUnitCount CounterUnit = "Count"
)
