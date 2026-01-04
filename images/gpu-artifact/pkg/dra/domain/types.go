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

package domain

import "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"

// InventorySnapshot is a minimal snapshot of node-scoped inventory.
type InventorySnapshot struct {
	NodeName  string
	NodeUID   string
	Inventory allocatable.Inventory
}

// AllocatedDevice references a device selected for a claim request.
type AllocatedDevice struct {
	Request string
	Driver  string
	Pool    string
	Device  string
}

// NodeSelector pins an allocation to a specific node.
type NodeSelector struct {
	NodeName string
}

// AllocationResult is a minimal allocation result representation.
type AllocationResult struct {
	ClaimUID     string
	NodeName     string
	Devices      []AllocatedDevice
	NodeSelector *NodeSelector
}

// PrepareRequest is a minimal prepare/unprepare request representation.
type PrepareRequest struct {
	ClaimUID string
	NodeName string
	Devices  []AllocatedDevice
}
