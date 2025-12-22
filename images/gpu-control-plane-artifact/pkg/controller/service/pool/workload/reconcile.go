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

package workload

import (
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/cleanup"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/deps"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/deviceplugin"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/migmanager"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/validator"
)

// Reconcile ensures per-pool workloads (device-plugin, MIG manager, validator) are deployed.
func Reconcile(ctx context.Context, d deps.Deps, pool *v1alpha1.GPUPool) (reconcile.Result, error) {
	if d.Client == nil {
		return reconcile.Result{}, fmt.Errorf("client is required")
	}
	if d.Config.Namespace == "" {
		return reconcile.Result{}, fmt.Errorf("namespace is not configured")
	}
	if d.Config.DevicePluginImage == "" {
		return reconcile.Result{}, fmt.Errorf("device-plugin image is not configured")
	}

	// Only Nvidia/DevicePlugin supported for now.
	if pool.Spec.Provider != "" && pool.Spec.Provider != "Nvidia" {
		return reconcile.Result{}, nil
	}
	if pool.Spec.Backend != "" && pool.Spec.Backend != "DevicePlugin" {
		return reconcile.Result{}, cleanup.PoolResources(ctx, d.Client, d.Config.Namespace, pool.Name)
	}
	if pool.Status.Capacity.Total == 0 {
		hasDevices, err := deviceplugin.PoolHasAssignedDevices(ctx, d, pool)
		if err != nil {
			return reconcile.Result{}, err
		}
		if !hasDevices {
			return reconcile.Result{}, cleanup.PoolResources(ctx, d.Client, d.Config.Namespace, pool.Name)
		}
	}
	if err := deviceplugin.Reconcile(ctx, d, pool); err != nil {
		return reconcile.Result{}, err
	}
	if err := validator.Reconcile(ctx, d, pool); err != nil {
		return reconcile.Result{}, err
	}

	if strings.EqualFold(pool.Spec.Resource.Unit, "MIG") {
		if d.Config.MIGManagerImage == "" {
			d.Log.Info("MIG pool detected but MIG manager image not configured, skipping MIG manager reconcile", "pool", pool.Name)
		} else if err := migmanager.Reconcile(ctx, d, pool); err != nil {
			return reconcile.Result{}, err
		}
	} else {
		if err := cleanup.MIGResources(ctx, d.Client, d.Config.Namespace, pool.Name); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
