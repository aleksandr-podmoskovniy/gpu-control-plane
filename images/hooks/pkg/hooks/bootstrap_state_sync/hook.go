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

package bootstrap_state_sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"hooks/pkg/settings"
)

const (
	bootstrapStateSnapshot = "gpu-gpu-node-state"
	bootstrapStateFilter   = `{"name": .metadata.name, "status": .status}`
	physicalGPUSnapshot    = "gpu-physical-gpu"
	physicalGPUFilter      = `{"nodeName": .status.nodeInfo.nodeName, "vendorID": .status.pciInfo.vendor.id}`
)

type inventorySnapshot struct {
	Name   string         `json:"name"`
	Status snapshotStatus `json:"status"`
}

type snapshotStatus struct {
	Conditions []metav1.Condition `json:"conditions"`
}

type physicalSnapshot struct {
	NodeName string `json:"nodeName"`
	VendorID string `json:"vendorID"`
}

var _ = registry.RegisterFunc(&pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 21},
	Queue:        settings.ModuleQueue,
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:                         bootstrapStateSnapshot,
			APIVersion:                   "gpu.deckhouse.io/v1alpha1",
			Kind:                         "GPUNodeState",
			JqFilter:                     bootstrapStateFilter,
			ExecuteHookOnSynchronization: ptr.To(true),
			ExecuteHookOnEvents:          ptr.To(true),
			AllowFailure:                 ptr.To(true),
			WaitForSynchronization:       ptr.To(false),
		},
		{
			Name:                         physicalGPUSnapshot,
			APIVersion:                   "gpu.deckhouse.io/v1alpha1",
			Kind:                         "PhysicalGPU",
			JqFilter:                     physicalGPUFilter,
			ExecuteHookOnSynchronization: ptr.To(true),
			ExecuteHookOnEvents:          ptr.To(true),
			AllowFailure:                 ptr.To(true),
			WaitForSynchronization:       ptr.To(false),
		},
	},
}, handleBootstrapStateSync)

func handleBootstrapStateSync(_ context.Context, input *pkg.HookInput) error {
	stateSnaps := input.Snapshots.Get(bootstrapStateSnapshot)
	physicalSnaps := input.Snapshots.Get(physicalGPUSnapshot)
	if len(stateSnaps) == 0 && len(physicalSnaps) == 0 {
		input.Values.Remove(settings.InternalBootstrapStatePath)
		return nil
	}

	nodes := map[string]map[string]any{}
	componentNodes := map[string][]string{
		settings.BootstrapComponentValidator:           []string{},
		settings.BootstrapComponentGPUFeatureDiscovery: []string{},
		settings.BootstrapComponentDCGM:                []string{},
		settings.BootstrapComponentDCGMExporter:        []string{},
		"handler":                                      []string{},
	}

	nvidiaNodes := map[string]struct{}{}
	for _, snap := range physicalSnaps {
		var payload physicalSnapshot
		if err := snap.UnmarshalTo(&payload); err != nil {
			input.Logger.Info(fmt.Sprintf("bootstrap-state: skip PhysicalGPU snapshot: %v", err))
			continue
		}
		nodeName := strings.TrimSpace(payload.NodeName)
		if nodeName == "" {
			continue
		}
		if normalizePCIID(payload.VendorID) == "10de" {
			nvidiaNodes[nodeName] = struct{}{}
		}
	}

	for _, snap := range stateSnaps {
		var payload inventorySnapshot
		if err := snap.UnmarshalTo(&payload); err != nil {
			input.Logger.Info(fmt.Sprintf("bootstrap-state: skip inventory snapshot: %v", err))
			continue
		}
		nodeName := strings.TrimSpace(payload.Name)
		if nodeName == "" {
			continue
		}

		conds := payload.Status.Conditions

		entry := map[string]any{
			"inventoryComplete": conditionTrue(conds, "InventoryComplete"),
			"driverReady":       conditionTrue(conds, "DriverReady"),
			"toolkitReady":      conditionTrue(conds, "ToolkitReady"),
			"monitoringReady":   conditionTrue(conds, "MonitoringReady"),
			"readyForPooling":   conditionTrue(conds, "ReadyForPooling"),
			"workloadsDegraded": conditionTrue(conds, "WorkloadsDegraded"),
		}
		nodes[nodeName] = entry

		// Stage 2+: run discovery/monitoring only after driver/toolkit are ready.
		if conditionTrue(conds, "DriverReady") && conditionTrue(conds, "ToolkitReady") {
			componentNodes[settings.BootstrapComponentGPUFeatureDiscovery] = append(componentNodes[settings.BootstrapComponentGPUFeatureDiscovery], nodeName)
			componentNodes[settings.BootstrapComponentDCGM] = append(componentNodes[settings.BootstrapComponentDCGM], nodeName)
			componentNodes[settings.BootstrapComponentDCGMExporter] = append(componentNodes[settings.BootstrapComponentDCGMExporter], nodeName)
		}
	}

	validatorNodes := []string{}
	if len(nvidiaNodes) > 0 {
		for nodeName := range nvidiaNodes {
			validatorNodes = append(validatorNodes, nodeName)
		}
		componentNodes["handler"] = append(componentNodes["handler"], validatorNodes...)
		componentNodes[settings.BootstrapComponentDCGM] = append([]string{}, validatorNodes...)
		componentNodes[settings.BootstrapComponentDCGMExporter] = append([]string{}, validatorNodes...)
	} else {
		for nodeName := range nodes {
			validatorNodes = append(validatorNodes, nodeName)
		}
	}
	componentNodes[settings.BootstrapComponentValidator] = validatorNodes

	if len(nodes) == 0 && len(nvidiaNodes) == 0 {
		input.Values.Remove(settings.InternalBootstrapStatePath)
		return nil
	}

	components := map[string]any{}
	componentHashes := make([]string, 0, len(componentNodes))

	for component, hostnames := range componentNodes {
		sort.Strings(hostnames)
		hash := hashStrings(hostnames)
		componentHashes = append(componentHashes, fmt.Sprintf("%s:%s", component, hash))
		nodesList := []string{}
		nodesList = append(nodesList, hostnames...)
		components[component] = map[string]any{
			"nodes": nodesList,
			"hash":  hash,
		}
	}

	sort.Strings(componentHashes)
	stateHash := hashStrings(componentHashes)

	input.Values.Set(settings.InternalBootstrapStatePath, map[string]any{
		"nodes":      nodes,
		"components": components,
		"version":    stateHash,
	})

	return nil
}

func conditionTrue(conditions []metav1.Condition, condType string) bool {
	for _, cond := range conditions {
		if cond.Type != condType {
			continue
		}
		return cond.Status == metav1.ConditionTrue
	}
	return false
}

func hashStrings(items []string) string {
	joined := strings.Join(items, ",")
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}

func normalizePCIID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimPrefix(trimmed, "0x")
	return strings.ToLower(trimmed)
}
