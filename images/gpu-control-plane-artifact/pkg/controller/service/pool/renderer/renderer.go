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

package renderer

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/cleanup"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/deps"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/deviceplugin"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/migmanager"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/validator"
)

// RendererHandler ensures per-pool workloads (device-plugin, MIG manager) are deployed.
type RendererHandler struct {
	log               logr.Logger
	client            client.Client
	cfg               RenderConfig
	customTolerations []corev1.Toleration
}

func (h *RendererHandler) Name() string {
	return "renderer"
}

func (h *RendererHandler) deps() deps.Deps {
	return deps.Deps{
		Log:               h.log,
		Client:            h.client,
		Config:            h.cfg,
		CustomTolerations: h.customTolerations,
	}
}

func (h *RendererHandler) HandlePool(ctx context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	if h.client == nil {
		return contracts.Result{}, fmt.Errorf("client is required")
	}
	if h.cfg.Namespace == "" {
		return contracts.Result{}, fmt.Errorf("namespace is not configured")
	}
	if h.cfg.DevicePluginImage == "" {
		return contracts.Result{}, fmt.Errorf("device-plugin image is not configured")
	}

	// Only Nvidia/DevicePlugin supported for now.
	if pool.Spec.Provider != "" && pool.Spec.Provider != "Nvidia" {
		return contracts.Result{}, nil
	}
	if pool.Spec.Backend != "" && pool.Spec.Backend != "DevicePlugin" {
		return contracts.Result{}, cleanup.PoolResources(ctx, h.client, h.cfg.Namespace, pool.Name)
	}
	d := h.deps()
	if pool.Status.Capacity.Total == 0 {
		hasDevices, err := deviceplugin.PoolHasAssignedDevices(ctx, d, pool)
		if err != nil {
			return contracts.Result{}, err
		}
		if !hasDevices {
			return contracts.Result{}, cleanup.PoolResources(ctx, h.client, h.cfg.Namespace, pool.Name)
		}
	}
	if err := deviceplugin.Reconcile(ctx, d, pool); err != nil {
		return contracts.Result{}, err
	}
	if err := validator.Reconcile(ctx, d, pool); err != nil {
		return contracts.Result{}, err
	}

	if strings.EqualFold(pool.Spec.Resource.Unit, "MIG") {
		if h.cfg.MIGManagerImage == "" {
			h.log.Info("MIG pool detected but MIG manager image not configured, skipping MIG manager rendering", "pool", pool.Name)
		} else if err := migmanager.Reconcile(ctx, d, pool); err != nil {
			return contracts.Result{}, err
		}
	} else {
		if err := cleanup.MIGResources(ctx, h.client, h.cfg.Namespace, pool.Name); err != nil {
			return contracts.Result{}, err
		}
	}

	return contracts.Result{}, nil
}
