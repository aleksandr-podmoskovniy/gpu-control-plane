//go:build linux
// +build linux

/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package timeslicing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	configapi "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/configapi"
)

// SetTimeSlice applies a time-slice interval to the specified GPUs.
func (m *Manager) SetTimeSlice(ctx context.Context, deviceUUIDs []string, cfg *configapi.TimeSlicingConfig) error {
	if m == nil || m.nvidiaSMIPath == "" {
		return fmt.Errorf("nvidia-smi path is not configured")
	}
	if cfg == nil || cfg.Interval == nil {
		return fmt.Errorf("time-slicing interval is not set")
	}
	interval := cfg.Interval.Int()
	if interval < 0 {
		return fmt.Errorf("unsupported time-slice interval %q", *cfg.Interval)
	}

	seen := map[string]struct{}{}
	for _, uuid := range deviceUUIDs {
		uuid = strings.TrimSpace(uuid)
		if uuid == "" {
			continue
		}
		if _, ok := seen[uuid]; ok {
			continue
		}
		if err := m.run(ctx, "-i", uuid, "-c", defaultComputeMode); err != nil {
			return fmt.Errorf("set compute mode for %q: %w", uuid, err)
		}
		if err := m.run(ctx, "compute-policy", "-i", uuid, "--set-timeslice", fmt.Sprintf("%d", interval)); err != nil {
			return fmt.Errorf("set time-slice for %q: %w", uuid, err)
		}
		seen[uuid] = struct{}{}
	}
	return nil
}

func (m *Manager) run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, m.nvidiaSMIPath, args...)
	cmd.Env = withLDPreload(os.Environ(), m.nvmlLibPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nvidia-smi failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
