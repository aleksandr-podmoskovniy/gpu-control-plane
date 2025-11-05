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

package ca_discovery

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"

	"hooks/pkg/settings"
)

const (
	commonCASecretSnapshot = "gpu-control-plane-root-ca"
	commonCASecretFilter   = `{
		"crt": .data."tls.crt",
		"key": .data."tls.key"
	}`
)

type caSecret struct {
	Crt string `json:"crt"`
	Key string `json:"key"`
}

var _ = registry.RegisterFunc(configModuleCommonCA, handleModuleCommonCA)

var configModuleCommonCA = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 1},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:                         commonCASecretSnapshot,
			APIVersion:                   "v1",
			Kind:                         "Secret",
			JqFilter:                     commonCASecretFilter,
			ExecuteHookOnSynchronization: ptr.To(false),
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{settings.RootCASecretName},
			},
			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{settings.ModuleNamespace},
				},
			},
		},
	},
	Queue: settings.ModuleQueue,
}

func handleModuleCommonCA(_ context.Context, input *pkg.HookInput) error {
	snapshots := input.Snapshots.Get(commonCASecretSnapshot)
	if len(snapshots) == 0 {
		input.Logger.Info("[ModuleCommonCA] No pre-existing GPU Control Plane CA secret; TLS hook will generate it if necessary.")
		return nil
	}

	var secret caSecret
	if err := snapshots[0].UnmarshalTo(&secret); err != nil {
		return fmt.Errorf("unmarshal CA secret: %w", err)
	}

	decoded := map[string]any{}
	if crt := decodeMaybeBase64(secret.Crt); crt != "" {
		decoded["crt"] = crt
	}
	if key := decodeMaybeBase64(secret.Key); key != "" {
		decoded["key"] = key
	}

	input.Values.Set(settings.InternalRootCAPath, decoded)

	return nil
}

func decodeMaybeBase64(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if data, err := base64.StdEncoding.DecodeString(raw); err == nil {
		trimmed := strings.TrimSpace(string(data))
		if strings.HasPrefix(trimmed, "-----BEGIN") && strings.HasSuffix(trimmed, "-----END") {
			return trimmed
		}
		return string(data)
	}

	return raw
}
