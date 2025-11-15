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
	"fmt"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"

	"hooks/pkg/settings"
)

const (
	rootCASecretSnapshot = "gpu-control-plane-root-ca"
	rootCASecretFilter   = `{
		"crt": .data."tls.crt",
		"key": .data."tls.key"
	}`
)

type caSecret struct {
	Crt []byte `json:"crt"`
	Key []byte `json:"key"`
}

var _ = registry.RegisterFunc(moduleCommonCAConfig, handleModuleCommonCA)

var moduleCommonCAConfig = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 1},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:                         rootCASecretSnapshot,
			APIVersion:                   "v1",
			Kind:                         "Secret",
			JqFilter:                     rootCASecretFilter,
			ExecuteHookOnSynchronization: ptr.To(false),
			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{settings.ModuleNamespace},
				},
			},
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{settings.RootCASecretName},
			},
		},
	},
	Queue: settings.ModuleQueue,
}

func handleModuleCommonCA(_ context.Context, input *pkg.HookInput) error {
	snapshots := input.Snapshots.Get(rootCASecretSnapshot)
	if len(snapshots) == 0 {
		input.Logger.Info("[ModuleCommonCA] No pre-existing GPU Control Plane CA secret; TLS hook will generate it if necessary.")
		return nil
	}

	var secret caSecret
	if err := snapshots[0].UnmarshalTo(&secret); err != nil {
		return fmt.Errorf("unmarshal CA secret: %w", err)
	}

	if len(secret.Crt) == 0 && len(secret.Key) == 0 {
		input.Values.Remove(settings.InternalRootCAPath)
		return nil
	}

	input.Values.Set(settings.InternalRootCAPath, secret)

	return nil
}
