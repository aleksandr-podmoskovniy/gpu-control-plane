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
	"fmt"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/deps"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/ops"
)

// Reconcile ensures the device plugin ConfigMap and DaemonSet are up to date.
func Reconcile(ctx context.Context, d deps.Deps, pool *v1alpha1.GPUPool) error {
	cm := devicePluginConfigMap(ctx, d, pool)
	if err := ops.CreateOrUpdate(ctx, d.Client, cm, pool); err != nil {
		return fmt.Errorf("reconcile device-plugin ConfigMap: %w", err)
	}

	ds := devicePluginDaemonSet(ctx, d, pool)
	configHash := sha256Hex(cm.Data["config.yaml"])
	if ds.Spec.Template.Annotations == nil {
		ds.Spec.Template.Annotations = map[string]string{}
	}
	ds.Spec.Template.Annotations["gpu.deckhouse.io/device-plugin-config-hash"] = configHash
	if err := ops.CreateOrUpdate(ctx, d.Client, ds, pool); err != nil {
		return fmt.Errorf("reconcile device-plugin DaemonSet: %w", err)
	}

	return nil
}
