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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"

	"hooks/pkg/settings"
)

const (
	bootstrapStateSnapshot = "gpu-bootstrap-state"
	bootstrapStateFilter   = `{"data": .data}`
)

type configMapSnapshot struct {
	Data map[string]string `json:"data"`
}

type nodeState struct {
	Phase      string          `json:"phase"`
	Components map[string]bool `json:"components"`
	UpdatedAt  metav1.Time     `json:"updatedAt"`
}

var (
	yamlUnmarshal = yaml.Unmarshal
)

var _ = registry.RegisterFunc(&pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 21},
	Queue:        settings.ModuleQueue,
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       bootstrapStateSnapshot,
			APIVersion: "v1",
			Kind:       "ConfigMap",
			JqFilter:   bootstrapStateFilter,
			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{settings.ModuleNamespace},
				},
			},
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{settings.BootstrapStateConfigMapName},
			},
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

	var cm configMapSnapshot
	if err := snaps[0].UnmarshalTo(&cm); err != nil {
		return fmt.Errorf("decode bootstrap state snapshot: %w", err)
	}

	if len(cm.Data) == 0 {
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

	for key, raw := range cm.Data {
		if key == "" || !strings.HasSuffix(key, settings.BootstrapStateNodeSuffix) {
			continue
		}
		nodeName := strings.TrimSuffix(key, settings.BootstrapStateNodeSuffix)
		if nodeName == "" {
			continue
		}

		var state nodeState
		if err := yamlUnmarshal([]byte(raw), &state); err != nil {
			input.Logger.Info(fmt.Sprintf("bootstrap-state: skip node %q: %v", nodeName, err))
			continue
		}

		entry := map[string]any{
			"phase": state.Phase,
		}
		if len(state.Components) > 0 {
			entry["components"] = state.Components
		}
		if !state.UpdatedAt.IsZero() {
			entry["updatedAt"] = state.UpdatedAt.Time.UTC().Format(time.RFC3339Nano)
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

func hashStrings(items []string) string {
	joined := strings.Join(items, ",")
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}
