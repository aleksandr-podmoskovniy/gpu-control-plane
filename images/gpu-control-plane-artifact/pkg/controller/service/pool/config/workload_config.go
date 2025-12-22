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

package config

import (
	"os"
	"strings"
)

// WorkloadConfig carries per-pool workload settings.
type WorkloadConfig struct {
	Namespace            string
	DevicePluginImage    string
	MIGManagerImage      string
	DefaultMIGStrategy   string
	CustomTolerationKeys []string
	ValidatorImage       string
}

// DefaultsFromEnv reads environment defaults for workload settings.
func DefaultsFromEnv() WorkloadConfig {
	ns := strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	if ns == "" {
		ns = "d8-gpu-control-plane"
	}
	strategy := strings.TrimSpace(os.Getenv("DEFAULT_MIG_STRATEGY"))
	if strategy == "" {
		strategy = "none"
	}
	return WorkloadConfig{
		Namespace:          ns,
		DevicePluginImage:  strings.TrimSpace(os.Getenv("NVIDIA_DEVICE_PLUGIN_IMAGE")),
		MIGManagerImage:    strings.TrimSpace(os.Getenv("NVIDIA_MIG_MANAGER_IMAGE")),
		DefaultMIGStrategy: strategy,
		ValidatorImage:     strings.TrimSpace(os.Getenv("NVIDIA_VALIDATOR_IMAGE")),
	}
}
