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

// PrepareCheckpoint stores node-local prepare state for idempotency.
type PrepareCheckpoint struct {
	Version string                   `json:"version"`
	Claims  map[string]PreparedClaim `json:"claims"`
}

// PrepareState indicates the state of claim preparation.
type PrepareState string

const (
	PrepareStateStarted   PrepareState = "PrepareStarted"
	PrepareStateCompleted PrepareState = "PrepareCompleted"
)

// PreparedClaim keeps prepared devices for a claim.
type PreparedClaim struct {
	State   PrepareState          `json:"state"`
	Devices []PreparedDeviceState `json:"devices,omitempty"`
}

// PreparedDeviceState stores data required to return and cleanup prepared devices.
type PreparedDeviceState struct {
	Request      string              `json:"request"`
	Pool         string              `json:"pool"`
	Device       string              `json:"device"`
	CDIDeviceIDs []string            `json:"cdiDeviceIDs,omitempty"`
	MIG          *PreparedMigDevice  `json:"mig,omitempty"`
	VFIO         *PreparedVfioDevice `json:"vfio,omitempty"`
}

// MigPrepareRequest describes a MIG instance to be created.
type MigPrepareRequest struct {
	PCIBusID   string `json:"pciBusId"`
	ProfileID  int    `json:"profileId"`
	SliceStart int    `json:"sliceStart"`
	SliceSize  int    `json:"sliceSize"`
}

// PreparedMigDevice stores created MIG instance details.
type PreparedMigDevice struct {
	PCIBusID          string `json:"pciBusId"`
	ProfileID         int    `json:"profileId"`
	SliceStart        int    `json:"sliceStart"`
	SliceSize         int    `json:"sliceSize"`
	GPUInstanceID     int    `json:"gpuInstanceId"`
	ComputeInstanceID int    `json:"computeInstanceId"`
	DeviceUUID        string `json:"deviceUuid"`
}

// VfioPrepareRequest describes a VFIO binding request.
type VfioPrepareRequest struct {
	PCIBusID string `json:"pciBusId"`
}

// PreparedVfioDevice stores VFIO binding metadata.
type PreparedVfioDevice struct {
	PCIBusID       string `json:"pciBusId"`
	OriginalDriver string `json:"originalDriver"`
	IommuGroup     int    `json:"iommuGroup"`
}
