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
	"time"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"hooks/pkg/settings"
)

const (
	bootstrapStateSnapshot = "gpu-gpu-node-inventory"
	bootstrapStateFilter   = `{"name": .metadata.name, "status": .status}`
)

type inventorySnapshot struct {
	Name   string                  `json:"name"`
	Status inventorySnapshotStatus `json:"status"`
}

type inventorySnapshotStatus struct {
	Bootstrap inventoryBootstrapStatus `json:"bootstrap"`
}

type inventoryBootstrapStatus struct {
	Phase             string          `json:"phase"`
	Components        map[string]bool `json:"components"`
	LastRun           *metav1.Time    `json:"lastRun"`
	ValidatorRequired bool            `json:"validatorRequired"`
	PendingDevices    []string        `json:"pendingDevices"`
}

var _ = registry.RegisterFunc(&pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 21},
	Queue:        settings.ModuleQueue,
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:                         bootstrapStateSnapshot,
			APIVersion:                   "gpu.deckhouse.io/v1alpha1",
			Kind:                         "GPUNodeInventory",
			JqFilter:                     bootstrapStateFilter,
			ExecuteHookOnSynchronization: ptr.To(true),
			ExecuteHookOnEvents:          ptr.To(true),
			AllowFailure:                 ptr.To(true),
			WaitForSynchronization:       ptr.To(false),
		},
	},
}, handleBootstrapStateSync)

func handleBootstrapStateSync(_ context.Context, input *pkg.HookInput) error {
	snaps := input.Snapshots.Get(bootstrapStateSnapshot)
	if len(snaps) == 0 {
		input.Values.Remove(settings.InternalBootstrapStatePath)
		return nil
	}

	nodes := map[string]map[string]any{}
	componentNodes := map[string][]string{
		settings.BootstrapComponentValidator:           {},
		settings.BootstrapComponentGPUFeatureDiscovery: {},
		settings.BootstrapComponentDCGM:                {},
		settings.BootstrapComponentDCGMExporter:        {},
	}

	for _, snap := range snaps {
		var payload inventorySnapshot
		if err := snap.UnmarshalTo(&payload); err != nil {
			input.Logger.Info(fmt.Sprintf("bootstrap-state: skip inventory snapshot: %v", err))
			continue
		}
		nodeName := strings.TrimSpace(payload.Name)
		if nodeName == "" {
			continue
		}
		state := payload.Status.Bootstrap
		if state.Phase == "" && len(state.Components) == 0 && state.LastRun == nil {
			continue
		}

		entry := map[string]any{}
		if state.Phase != "" {
			entry["phase"] = state.Phase
		}
		if len(state.Components) > 0 {
			entry["components"] = copyComponents(state.Components)
		}
		if state.LastRun != nil && !state.LastRun.IsZero() {
			entry["updatedAt"] = state.LastRun.Time.UTC().Format(time.RFC3339Nano)
		}
		if state.ValidatorRequired {
			entry["validatorRequired"] = true
		}
		if len(state.PendingDevices) > 0 {
			entry["pendingDevices"] = append([]string(nil), state.PendingDevices...)
		}
		if len(entry) == 0 {
			continue
		}
		nodes[nodeName] = entry

		for component, enabled := range state.Components {
			if !enabled {
				continue
			}
			if _, ok := componentNodes[component]; !ok {
				componentNodes[component] = []string{}
			}
			componentNodes[component] = append(componentNodes[component], nodeName)
		}
	}

	if len(nodes) == 0 {
		input.Values.Remove(settings.InternalBootstrapStatePath)
		return nil
	}

	components := map[string]any{}
	componentHashes := make([]string, 0, len(componentNodes))

	for component, hostnames := range componentNodes {
		sort.Strings(hostnames)
		hash := hashStrings(hostnames)
		componentHashes = append(componentHashes, fmt.Sprintf("%s:%s", component, hash))
		components[component] = map[string]any{
			"nodes": append([]string(nil), hostnames...),
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

func copyComponents(components map[string]bool) map[string]bool {
	if len(components) == 0 {
		return nil
	}
	out := make(map[string]bool, len(components))
	for key, value := range components {
		out[key] = value
	}
	return out
}

func hashStrings(items []string) string {
	joined := strings.Join(items, ",")
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}
