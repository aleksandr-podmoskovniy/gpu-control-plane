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

package inventory

import invtypes "github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/service/inventory/types"

// Type aliases keep the public inventory API stable while splitting packages.
type (
	BuildContext        = invtypes.BuildContext
	BuildResult         = invtypes.BuildResult
	DeviceBuilder       = invtypes.DeviceBuilder
	InventoryBuilder    = invtypes.InventoryBuilder
	Plan                = invtypes.Plan
	MigPlacement        = invtypes.MigPlacement
	MigPlacementReader  = invtypes.MigPlacementReader
	MigPlacementSession = invtypes.MigPlacementSession
)
