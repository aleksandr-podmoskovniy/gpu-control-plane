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

package mps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	configapi "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/configapi"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// Start launches a control daemon if it is not already running.
func (m *Manager) Start(ctx context.Context, req domain.MpsPrepareRequest) (domain.PreparedMpsState, error) {
	if m == nil {
		return domain.PreparedMpsState{}, errors.New("mps manager is nil")
	}
	if m.controlBinary == "" {
		return domain.PreparedMpsState{}, errors.New("mps control binary is not configured")
	}
	if m.pluginPath == "" {
		return domain.PreparedMpsState{}, errors.New("plugin path is not configured")
	}
	controlID := strings.TrimSpace(req.ControlID)
	if controlID == "" {
		return domain.PreparedMpsState{}, errors.New("mps control id is empty")
	}
	deviceUUIDs := uniqueStrings(req.DeviceUUIDs)
	if len(deviceUUIDs) == 0 {
		return domain.PreparedMpsState{}, errors.New("no device UUIDs provided for MPS")
	}

	rootDir := filepath.Join(m.pluginPath, defaultControlDir, controlID)
	pipeDir := filepath.Join(rootDir, defaultPipeDir)
	logDir := filepath.Join(rootDir, defaultLogDir)

	if err := ensureDir(pipeDir); err != nil {
		return domain.PreparedMpsState{}, fmt.Errorf("create MPS pipe dir: %w", err)
	}
	if err := ensureDir(logDir); err != nil {
		return domain.PreparedMpsState{}, fmt.Errorf("create MPS log dir: %w", err)
	}

	startupLog := filepath.Join(logDir, startupLogName)
	if exists(startupLog) {
		return domain.PreparedMpsState{
			ControlID: controlID,
			PipeDir:   pipeDir,
			LogDir:    logDir,
			ShmDir:    m.shmDir,
		}, nil
	}

	cmd := exec.CommandContext(ctx, m.controlBinary, "-d")
	cmd.Env = m.controlEnv(deviceUUIDs, pipeDir, logDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return domain.PreparedMpsState{}, fmt.Errorf("start MPS control daemon: %s", strings.TrimSpace(string(output)))
	}

	if err := m.applyConfig(ctx, deviceUUIDs, pipeDir, logDir, req.Config); err != nil {
		return domain.PreparedMpsState{}, err
	}

	if err := os.WriteFile(startupLog, []byte("startup complete\n"), 0o644); err != nil {
		return domain.PreparedMpsState{}, fmt.Errorf("write MPS startup log: %w", err)
	}

	return domain.PreparedMpsState{
		ControlID: controlID,
		PipeDir:   pipeDir,
		LogDir:    logDir,
		ShmDir:    m.shmDir,
	}, nil
}

func (m *Manager) applyConfig(ctx context.Context, deviceUUIDs []string, pipeDir, logDir string, cfg *configapi.MpsConfig) error {
	if cfg == nil {
		return nil
	}
	if cfg.DefaultActiveThreadPercentage != nil {
		cmd := fmt.Sprintf("set_default_active_thread_percentage %d", *cfg.DefaultActiveThreadPercentage)
		if err := m.runCommand(ctx, pipeDir, logDir, cmd); err != nil {
			return err
		}
	}

	limits, err := cfg.DefaultPerDevicePinnedMemoryLimit.Normalize(deviceUUIDs, cfg.DefaultPinnedDeviceMemoryLimit)
	if err != nil {
		return fmt.Errorf("normalize MPS memory limits: %w", err)
	}
	limitKeys := make([]string, 0, len(limits))
	for uuid := range limits {
		limitKeys = append(limitKeys, uuid)
	}
	sort.Strings(limitKeys)
	for _, uuid := range limitKeys {
		limit := limits[uuid]
		cmd := fmt.Sprintf("set_default_device_pinned_mem_limit %s %s", uuid, limit)
		if err := m.runCommand(ctx, pipeDir, logDir, cmd); err != nil {
			return err
		}
	}
	return nil
}
