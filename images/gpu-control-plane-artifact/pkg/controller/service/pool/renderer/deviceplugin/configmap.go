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

package deviceplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	poolsvc "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/deps"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/names"
)

func devicePluginConfigMap(ctx context.Context, d deps.Deps, pool *v1alpha1.GPUPool) *corev1.ConfigMap {
	resourceName := names.ResolveResourceName(pool, pool.Name)
	replicas := timeSlicingReplicas(pool)
	patterns := AssignedDevicePatterns(ctx, d, pool)

	hasSharing := replicas > 1
	resources := []map[string]any{{
		"name":     resourceName,
		"replicas": int(replicas),
	}}

	cfg := map[string]any{
		"version": "v1",
		"flags": map[string]any{
			"migStrategy":    d.Config.DefaultMIGStrategy,
			"resourcePrefix": poolsvc.PoolResourcePrefixFor(pool),
		},
		"plugin": map[string]any{
			"passDeviceSpecs":    true,
			"deviceListStrategy": "envvar",
			"deviceIDStrategy":   "uuid",
		},
	}

	gpus := make([]map[string]any, 0, len(patterns))
	if len(patterns) == 0 {
		gpus = append(gpus, map[string]any{
			// Avoid exposing all GPUs when device identifiers are not available yet.
			"pattern": "^$",
			"name":    resourceName,
		})
	} else {
		for _, p := range patterns {
			gpus = append(gpus, map[string]any{
				"pattern": p,
				"name":    resourceName,
			})
		}
	}

	cfg["resources"] = map[string]any{
		"gpus": gpus,
	}

	if hasSharing {
		cfg["sharing"] = map[string]any{
			"timeSlicing": map[string]any{
				"resources": resources,
			},
		}
	}

	data, _ := yaml.Marshal(cfg)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-device-plugin-%s-config", pool.Name),
			Namespace: d.Config.Namespace,
			Labels: map[string]string{
				"app":  "nvidia-device-plugin",
				"pool": pool.Name,
			},
		},
		Data: map[string]string{"config.yaml": string(data)},
	}
}

func sha256Hex(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

func timeSlicingReplicas(pool *v1alpha1.GPUPool) int32 {
	replicas := int32(1)
	if pool.Spec.Resource.SlicesPerUnit > 0 {
		replicas = pool.Spec.Resource.SlicesPerUnit
	}
	return replicas
}

func normalisePatterns(patterns map[string]struct{}) []string {
	out := make([]string, 0, len(patterns))
	for p := range patterns {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func trimUUID(uuid string) string {
	return strings.TrimSpace(uuid)
}
