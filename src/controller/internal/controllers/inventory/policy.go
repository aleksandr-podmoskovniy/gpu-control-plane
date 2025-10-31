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

package inventory

import (
	"fmt"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
)

type ManagedNodesPolicy struct {
	LabelKey         string
	EnabledByDefault bool
}

type DeviceApprovalPolicy struct {
	mode     config.DeviceApprovalMode
	selector labels.Selector
}

const defaultManagedNodeLabelKey = "gpu.deckhouse.io/enabled"

func newDeviceApprovalPolicy(cfg config.DeviceApprovalSettings) (DeviceApprovalPolicy, error) {
	policy := DeviceApprovalPolicy{mode: cfg.Mode}
	if policy.mode == "" {
		policy.mode = config.DeviceApprovalModeManual
	}

	switch policy.mode {
	case config.DeviceApprovalModeManual, config.DeviceApprovalModeAutomatic:
		return policy, nil
	case config.DeviceApprovalModeSelector:
		selector := cfg.Selector
		if selector == nil {
			selector = &metav1.LabelSelector{}
		}
		compiled, err := metav1.LabelSelectorAsSelector(selector)
		if err != nil {
			return DeviceApprovalPolicy{}, fmt.Errorf("compile device approval selector: %w", err)
		}
		policy.selector = compiled
		return policy, nil
	default:
		policy.mode = config.DeviceApprovalModeManual
		return policy, nil
	}
}

func (p DeviceApprovalPolicy) AutoAttach(managed bool, labels labels.Set) bool {
	if !managed {
		return false
	}

	switch p.mode {
	case config.DeviceApprovalModeAutomatic:
		return true
	case config.DeviceApprovalModeSelector:
		if p.selector == nil {
			return false
		}
		return p.selector.Matches(labels)
	default:
		return false
	}
}

func labelsForDevice(snapshot deviceSnapshot, nodeLabels map[string]string) labels.Set {
	result := labels.Set{}

	// Preserve per-device labels (with index) from Node/NodeFeature.
	prefix := deviceLabelPrefix + snapshot.Index + "."
	for key, value := range nodeLabels {
		if strings.HasPrefix(key, prefix) {
			result[key] = value
		}
	}

	// Aggregate a few commonly used fields without the index suffix for convenience.
	result["gpu.deckhouse.io/device.index"] = snapshot.Index
	if snapshot.Vendor != "" {
		result["gpu.deckhouse.io/device.vendor"] = strings.ToLower(snapshot.Vendor)
	}
	if snapshot.Device != "" {
		result["gpu.deckhouse.io/device.device"] = strings.ToLower(snapshot.Device)
	}
	if snapshot.Class != "" {
		result["gpu.deckhouse.io/device.class"] = strings.ToLower(snapshot.Class)
	}
	if snapshot.Product != "" {
		result["gpu.deckhouse.io/device.product"] = snapshot.Product
	}
	if snapshot.UUID != "" {
		result["gpu.deckhouse.io/device.uuid"] = snapshot.UUID
	}
	if snapshot.MemoryMiB > 0 {
		result["gpu.deckhouse.io/device.memoryMiB"] = strconv.Itoa(int(snapshot.MemoryMiB))
	}
	if snapshot.MIG.Capable {
		result["gpu.deckhouse.io/device.mig.capable"] = "true"
		if snapshot.MIG.Strategy != "" {
			result["gpu.deckhouse.io/device.mig.strategy"] = string(snapshot.MIG.Strategy)
		}
	} else {
		result["gpu.deckhouse.io/device.mig.capable"] = "false"
	}

	return result
}
