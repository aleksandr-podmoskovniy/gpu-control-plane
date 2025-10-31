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

package contracts

import (
	"context"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

// AdmissionHandler validates GPUPool state before it is handed to the webhook implementation.
type AdmissionHandler interface {
	Named
	SyncPool(ctx context.Context, pool *gpuv1alpha1.GPUPool) (Result, error)
}

// AdmissionRegistry stores admission handlers.
type AdmissionRegistry struct {
	*Registry[AdmissionHandler]
}

// NewAdmissionRegistry constructs an empty admission registry.
func NewAdmissionRegistry() *AdmissionRegistry {
	return &AdmissionRegistry{Registry: NewRegistry[AdmissionHandler]()}
}
