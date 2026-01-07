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

package prepare

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
)

func buildMigPrepareRequest(dev domain.PrepareDevice) (domain.MigPrepareRequest, error) {
	pci := attrString(dev.Attributes, allocatable.AttrPCIAddress)
	if pci == "" {
		return domain.MigPrepareRequest{}, fmt.Errorf("pci address is missing for device %q", dev.Device)
	}
	profileID, start, size, err := parseMigDeviceName(dev.Device)
	if err != nil {
		return domain.MigPrepareRequest{}, err
	}
	return domain.MigPrepareRequest{
		PCIBusID:   pci,
		ProfileID:  profileID,
		SliceStart: start,
		SliceSize:  size,
	}, nil
}

func parseMigDeviceName(name string) (int, int, int, error) {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "mig-") {
		return 0, 0, 0, fmt.Errorf("invalid mig device name %q", name)
	}
	parts := strings.SplitN(name, "-p", 2)
	if len(parts) != 2 {
		return 0, 0, 0, fmt.Errorf("invalid mig device name %q", name)
	}
	profilePart := parts[1]
	parts = strings.SplitN(profilePart, "-s", 2)
	if len(parts) != 2 {
		return 0, 0, 0, fmt.Errorf("invalid mig device name %q", name)
	}
	profileID, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid mig profile id in %q", name)
	}
	parts = strings.SplitN(parts[1], "-n", 2)
	if len(parts) != 2 {
		return 0, 0, 0, fmt.Errorf("invalid mig device name %q", name)
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid mig placement start in %q", name)
	}
	size, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid mig placement size in %q", name)
	}
	return profileID, start, size, nil
}
