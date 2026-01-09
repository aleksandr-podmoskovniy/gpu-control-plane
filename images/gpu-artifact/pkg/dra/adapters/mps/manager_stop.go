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
	"strings"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// Stop terminates a control daemon and cleans up its directories.
func (m *Manager) Stop(ctx context.Context, state domain.PreparedMpsState) error {
	if m == nil {
		return errors.New("mps manager is nil")
	}
	controlID := strings.TrimSpace(state.ControlID)
	if controlID == "" {
		return nil
	}

	pipeDir := strings.TrimSpace(state.PipeDir)
	logDir := strings.TrimSpace(state.LogDir)
	if pipeDir == "" || logDir == "" {
		rootDir := filepath.Join(m.pluginPath, defaultControlDir, controlID)
		if pipeDir == "" {
			pipeDir = filepath.Join(rootDir, defaultPipeDir)
		}
		if logDir == "" {
			logDir = filepath.Join(rootDir, defaultLogDir)
		}
	}

	if err := m.runCommand(ctx, pipeDir, logDir, "quit"); err != nil {
		return err
	}

	if m.pluginPath == "" {
		return nil
	}
	rootDir := filepath.Join(m.pluginPath, defaultControlDir, controlID)
	if err := os.RemoveAll(rootDir); err != nil {
		return fmt.Errorf("remove MPS control dir: %w", err)
	}
	return nil
}

func (m *Manager) runCommand(ctx context.Context, pipeDir, logDir, command string) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, m.controlBinary)
	cmd.Env = m.controlEnv(nil, pipeDir, logDir)
	cmd.Stdin = strings.NewReader(command + "\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("MPS command %q failed: %s", command, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) controlEnv(deviceUUIDs []string, pipeDir, logDir string) []string {
	env := os.Environ()
	if len(deviceUUIDs) > 0 {
		env = setEnv(env, "CUDA_VISIBLE_DEVICES", strings.Join(deviceUUIDs, ","))
	}
	if pipeDir != "" {
		env = setEnv(env, "CUDA_MPS_PIPE_DIRECTORY", pipeDir)
	}
	if logDir != "" {
		env = setEnv(env, "CUDA_MPS_LOG_DIRECTORY", logDir)
	}
	return withLDLibraryPath(env, m.driverRoot)
}
