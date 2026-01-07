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

import (
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

// InventorySnapshot is a minimal snapshot of node-scoped inventory.
type InventorySnapshot struct {
	NodeName  string
	NodeUID   string
	Inventory allocatable.Inventory
}

// AllocatedDevice references a device selected for a claim request.
type AllocatedDevice struct {
	Request                  string
	Driver                   string
	Pool                     string
	Device                   string
	ShareID                  string
	ConsumedCapacity         map[string]resource.Quantity
	BindingConditions        []string
	BindingFailureConditions []string
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

// PrepareDevice references a device that needs node preparation.
type PrepareDevice struct {
	Request          string
	Driver           string
	Pool             string
	Device           string
	ShareID          string
	ConsumedCapacity map[string]resource.Quantity
	Attributes       map[string]allocatable.AttributeValue
}

// PrepareRequest is a minimal prepare/unprepare request representation.
type PrepareRequest struct {
	ClaimUID string
	NodeName string
	VFIO     bool
	Devices  []PrepareDevice
}

// PreparedDevice represents a prepared device with CDI ids.
type PreparedDevice struct {
	Request      string
	Pool         string
	Device       string
	CDIDeviceIDs []string
}

// PrepareResult contains results for a single claim.
type PrepareResult struct {
	ClaimUID string
	Devices  []PreparedDevice
}
