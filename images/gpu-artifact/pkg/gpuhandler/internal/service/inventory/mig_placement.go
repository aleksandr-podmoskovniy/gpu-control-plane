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

// MigPlacementReader opens sessions for reading MIG placements.
type MigPlacementReader interface {
	Open() (MigPlacementSession, error)
}

// MigPlacementSession reads MIG placements for a device.
type MigPlacementSession interface {
	Close()
	ReadPlacements(pciAddress string, profileIDs []int32) (map[int32][]MigPlacement, error)
}

// MigPlacement describes a single MIG placement segment.
type MigPlacement struct {
	Start int32
	Size  int32
}
