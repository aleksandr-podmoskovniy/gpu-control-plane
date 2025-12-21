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

package migmanager

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/assets"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/deps"
)

func migManagerConfigMap(d deps.Deps, pool *v1alpha1.GPUPool) *corev1.ConfigMap {
	cfg := map[string]any{
		"version": 1,
		"mig-configs": []map[string]any{
			{
				"name": "default",
				"devices": []map[string]any{{
					"pciBusId":   "all",
					"migEnabled": true,
					"migDevices": []map[string]any{{"profile": pool.Spec.Resource.MIGProfile}},
				}},
			},
		},
	}

	data, _ := yaml.Marshal(cfg)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-mig-manager-%s-config", pool.Name),
			Namespace: d.Config.Namespace,
			Labels: map[string]string{
				"app":  "nvidia-mig-manager",
				"pool": pool.Name,
			},
		},
		Data: map[string]string{"config.yaml": string(data)},
	}
}

func migManagerScriptsConfigMap(d deps.Deps, pool *v1alpha1.GPUPool) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-mig-manager-%s-scripts", pool.Name),
			Namespace: d.Config.Namespace,
			Labels: map[string]string{
				"app":  "nvidia-mig-manager",
				"pool": pool.Name,
			},
		},
		Data: map[string]string{
			"reconfigure-mig.sh": assets.MIGReconfigureScript,
			"prestop.sh":         assets.MIGPrestopScript,
		},
	}
}

func migManagerClientsConfigMap(d deps.Deps, pool *v1alpha1.GPUPool) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-mig-manager-%s-gpu-clients", pool.Name),
			Namespace: d.Config.Namespace,
			Labels: map[string]string{
				"app":  "nvidia-mig-manager",
				"pool": pool.Name,
			},
		},
		Data: map[string]string{
			"clients.yaml": assets.MIGGPUClients,
		},
	}
}
