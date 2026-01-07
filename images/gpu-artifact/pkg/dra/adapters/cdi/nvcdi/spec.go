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

	nvcdispec "github.com/NVIDIA/nvidia-container-toolkit/pkg/nvcdi/spec"
	transformroot "github.com/NVIDIA/nvidia-container-toolkit/pkg/nvcdi/transform/root"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

func buildSpec(vendor, class string, deviceSpecs []cdispec.Device, edits *cdispec.ContainerEdits) (nvcdispec.Interface, error) {
	if edits == nil {
		return nil, errors.New("container edits are nil")
	}
	return nvcdispec.New(
		nvcdispec.WithVendor(vendor),
		nvcdispec.WithClass(class),
		nvcdispec.WithDeviceSpecs(deviceSpecs),
		nvcdispec.WithEdits(*edits),
	)
}

func (w *Writer) writeSpec(spec nvcdispec.Interface, specName string) error {
	if spec == nil {
		return errors.New("CDI spec is nil")
	}
	if err := transformroot.New(
		transformroot.WithRoot(w.driverRoot),
		transformroot.WithTargetRoot(w.targetDriverRoot),
		transformroot.WithRelativeTo("host"),
	).Transform(spec.Raw()); err != nil {
		return fmt.Errorf("transform CDI spec: %w", err)
	}

	minVersion, err := cdispec.MinimumRequiredVersion(spec.Raw())
	if err != nil {
		return fmt.Errorf("detect CDI spec version: %w", err)
	}
	spec.Raw().Version = minVersion

	if err := w.cache.WriteSpec(spec.Raw(), specName); err != nil {
		return fmt.Errorf("write CDI spec %q: %w", specName, err)
	}
	return nil
}
