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

package consts

import "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"

const (
	DeviceNodeIndexKey  = indexer.GPUDeviceNodeField
	DeviceLabelPrefix   = "gpu.deckhouse.io/device."
	DeviceNodeLabelKey  = "gpu.deckhouse.io/node"
	DeviceIndexLabelKey = "gpu.deckhouse.io/device-index"

	// DefaultManagedNodeLabelKey marks nodes enabled for GPU inventory management.
	DefaultManagedNodeLabelKey = "gpu.deckhouse.io/enabled"

	// NodeFeatureNodeNameLabel is NFD label with node name.
	NodeFeatureNodeNameLabel = "nfd.node.kubernetes.io/node-name"

	// Inventory condition and reasons.
	ConditionInventoryComplete = "InventoryComplete"
	ReasonInventorySynced      = "InventorySynced"
	ReasonNoDevicesDiscovered  = "NoDevicesDiscovered"
	ReasonNodeFeatureMissing   = "NodeFeatureMissing"

	// Inventory events.
	EventDeviceDetected    = "GPUDeviceDetected"
	EventDeviceRemoved     = "GPUDeviceRemoved"
	EventInventoryChanged  = "GPUInventoryConditionChanged"
	EventDetectUnavailable = "GPUDetectionUnavailable"

	// NFD/GFD labels.
	GFDProductLabel            = "nvidia.com/gpu.product"
	GFDMemoryLabel             = "nvidia.com/gpu.memory"
	GFDComputeMajorLabel       = "nvidia.com/gpu.compute.major"
	GFDComputeMinorLabel       = "nvidia.com/gpu.compute.minor"
	GFDDriverVersionLabel      = "nvidia.com/gpu.driver"
	GFDCudaRuntimeVersionLabel = "nvidia.com/cuda.runtime.version"
	GFDCudaDriverMajorLabel    = "nvidia.com/cuda.driver.major"
	GFDCudaDriverMinorLabel    = "nvidia.com/cuda.driver.minor"
	GFDMigCapableLabel         = "nvidia.com/mig.capable"
	GFDMigStrategyLabel        = "nvidia.com/mig.strategy"
	GFDMigAltCapableLabel      = "nvidia.com/mig-capable"
	GFDMigAltStrategyLabel     = "nvidia.com/mig-strategy"
	DeckhouseToolkitInstalled  = "gpu.deckhouse.io/toolkit.installed"
	DeckhouseToolkitReadyLabel = "gpu.deckhouse.io/toolkit.ready"

	MIGProfileLabelPrefix = "nvidia.com/mig-"
	VendorNvidia          = "10de"
)
