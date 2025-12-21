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
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/renderer/tolerations"
)

type RenderConfig = config.RenderConfig

// NewRendererHandler builds a RendererHandler using env fallbacks.
func NewRendererHandler(log logr.Logger, c client.Client, cfg RenderConfig) *RendererHandler {
	defaults := config.DefaultsFromEnv()
	if cfg.Namespace == "" {
		cfg.Namespace = defaults.Namespace
	}
	if cfg.DevicePluginImage == "" {
		cfg.DevicePluginImage = defaults.DevicePluginImage
	}
	if cfg.MIGManagerImage == "" {
		cfg.MIGManagerImage = defaults.MIGManagerImage
	}
	if cfg.DefaultMIGStrategy == "" {
		cfg.DefaultMIGStrategy = defaults.DefaultMIGStrategy
	}
	if cfg.ValidatorImage == "" {
		if defaults.ValidatorImage != "" {
			cfg.ValidatorImage = defaults.ValidatorImage
		} else {
			cfg.ValidatorImage = cfg.DevicePluginImage
		}
	}
	return &RendererHandler{
		log:               log,
		client:            c,
		cfg:               cfg,
		customTolerations: tolerations.Merge(nil, tolerations.BuildCustom(cfg.CustomTolerationKeys)),
	}
}
