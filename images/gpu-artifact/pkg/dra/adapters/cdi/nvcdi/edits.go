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

package nvcdi

import (
	"errors"
	"fmt"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
)

func (w *Writer) commonEdits() (*cdiapi.ContainerEdits, error) {
	commonEdits, err := w.nvcdi.GetCommonEdits()
	if err != nil {
		return nil, fmt.Errorf("get common CDI edits: %w", err)
	}
	if commonEdits == nil || commonEdits.ContainerEdits == nil {
		return nil, errors.New("common CDI edits are empty")
	}
	commonEdits.ContainerEdits.Env = append(commonEdits.ContainerEdits.Env, "NVIDIA_VISIBLE_DEVICES=void")
	return commonEdits, nil
}
