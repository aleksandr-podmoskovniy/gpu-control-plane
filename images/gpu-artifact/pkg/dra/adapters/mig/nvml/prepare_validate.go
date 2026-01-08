//go:build linux && cgo && nvml
// +build linux,cgo,nvml

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

package mig

import (
	"errors"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

func validatePrepareRequest(req domain.MigPrepareRequest) error {
	if req.PCIBusID == "" {
		return errors.New("pci bus id is required")
	}
	if req.SliceSize <= 0 {
		return errors.New("mig placement size is required")
	}
	return nil
}
