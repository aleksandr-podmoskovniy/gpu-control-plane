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
	"os"
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/config"
)

func TestApplyDefaultsUsesEnvFallbacks(t *testing.T) {
	env := map[string]string{
		"POD_NAMESPACE":              os.Getenv("POD_NAMESPACE"),
		"NVIDIA_DEVICE_PLUGIN_IMAGE": os.Getenv("NVIDIA_DEVICE_PLUGIN_IMAGE"),
		"NVIDIA_VALIDATOR_IMAGE":     os.Getenv("NVIDIA_VALIDATOR_IMAGE"),
	}
	t.Cleanup(func() {
		for k, v := range env {
			if v == "" {
				_ = os.Unsetenv(k)
				continue
			}
			_ = os.Setenv(k, v)
		}
	})

	_ = os.Setenv("POD_NAMESPACE", "env-ns")
	_ = os.Setenv("NVIDIA_DEVICE_PLUGIN_IMAGE", "dp:env")
	_ = os.Setenv("NVIDIA_VALIDATOR_IMAGE", "val:env")

	cfg := ApplyDefaults(config.WorkloadConfig{})
	if cfg.Namespace != "env-ns" {
		t.Fatalf("unexpected namespace: %q", cfg.Namespace)
	}
	if cfg.DevicePluginImage != "dp:env" {
		t.Fatalf("unexpected device-plugin image: %q", cfg.DevicePluginImage)
	}
	if cfg.ValidatorImage != "val:env" {
		t.Fatalf("unexpected validator image: %q", cfg.ValidatorImage)
	}
}
