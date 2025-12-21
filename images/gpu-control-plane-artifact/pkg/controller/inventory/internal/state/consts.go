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

package state

import invconsts "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/consts"

const (
	deviceLabelPrefix = invconsts.DeviceLabelPrefix

	gfdProductLabel            = invconsts.GFDProductLabel
	gfdMemoryLabel             = invconsts.GFDMemoryLabel
	gfdComputeMajorLabel       = invconsts.GFDComputeMajorLabel
	gfdComputeMinorLabel       = invconsts.GFDComputeMinorLabel
	gfdDriverVersionLabel      = invconsts.GFDDriverVersionLabel
	gfdCudaRuntimeVersionLabel = invconsts.GFDCudaRuntimeVersionLabel
	gfdCudaDriverMajorLabel    = invconsts.GFDCudaDriverMajorLabel
	gfdCudaDriverMinorLabel    = invconsts.GFDCudaDriverMinorLabel
	gfdMigCapableLabel         = invconsts.GFDMigCapableLabel
	gfdMigStrategyLabel        = invconsts.GFDMigStrategyLabel
	gfdMigAltCapableLabel      = invconsts.GFDMigAltCapableLabel
	gfdMigAltStrategy          = invconsts.GFDMigAltStrategyLabel
	deckhouseToolkitInstalled  = invconsts.DeckhouseToolkitInstalled
	deckhouseToolkitReadyLabel = invconsts.DeckhouseToolkitReadyLabel

	migProfileLabelPrefix = invconsts.MIGProfileLabelPrefix
	vendorNvidia          = invconsts.VendorNvidia
)

