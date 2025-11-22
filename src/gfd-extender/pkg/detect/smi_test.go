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

package detect

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestQueryNVMLSucceeds(t *testing.T) {
	execSmi = func(ctx context.Context) ([][]string, error) {
		return [][]string{
			{
				"GPU-123", "A100", "4096", "1024", "3072", "P0", "1200", "1100", "50", "60",
				"0000:01:00.0", "4", "16", "SERIAL123", "ampere", "65", "250", "535.104.06",
			},
			{
				"GPU-456", "B200", "0", "0", "0", "P2", "", "", "", "",
				"", "", "", "", "", "", "", "",
			},
		}, nil
	}
	t.Cleanup(func() { execSmi = runSmi })

	infos, err := queryNVML()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(infos))
	}
	first := infos[0]
	if first.UUID != "GPU-123" || first.Product != "A100" || first.MemoryMiB != 4096 {
		t.Fatalf("unexpected first info: %+v", first)
	}
	if first.PCI.Address != "0000:01:00.0" || first.PCIE.Generation == nil || *first.PCIE.Generation != 4 {
		t.Fatalf("unexpected pci info: %+v", first.PCI)
	}
	if first.PowerUsage == 0 {
		t.Fatalf("expected power usage parsed")
	}
	if len(first.Warnings) != 0 {
		t.Fatalf("expected no warnings: %+v", first.Warnings)
	}

	second := infos[1]
	if !second.Partial || len(second.Warnings) == 0 {
		t.Fatalf("expected partial data for second device")
	}
}

func TestQueryNVMLExecError(t *testing.T) {
	execSmi = func(ctx context.Context) ([][]string, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { execSmi = runSmi })

	if _, err := queryNVML(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestNormalizePCIAddress(t *testing.T) {
	tests := map[string]string{
		"0000:01:00.0": "0000:01:00.0",
		"01:00.0":      "0000:01:00.0",
		"":             "",
		"junk":         "",
	}
	for in, want := range tests {
		if got := normalizePCIAddress(in); got != want {
			t.Fatalf("expected %s -> %s, got %s", in, want, got)
		}
	}
}

func TestWarningsTruncated(t *testing.T) {
	origMax := maxWarningsPerGPU
	maxWarningsPerGPU = 1
	execSmi = func(ctx context.Context) ([][]string, error) {
		// second device will produce many missing fields
		return [][]string{
			{"GPU-1", "A100", "1024", "512", "512", "P0", "1100", "900", "10", "20", "0000:01:00.0", "4", "16", "SERIAL", "ampere", "60", "200", "535.104.06"},
			{"GPU-2"},
		}, nil
	}
	t.Cleanup(func() {
		execSmi = runSmi
		maxWarningsPerGPU = origMax
	})

	infos, err := queryNVML()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(infos))
	}
	if len(infos[1].Warnings) == 0 {
		t.Fatalf("expected warnings for second device")
	}
	if len(infos[1].Warnings) > maxWarningsPerGPU+1 { // +1 for truncated marker
		t.Fatalf("warnings should be truncated, got %d", len(infos[1].Warnings))
	}
}

func TestZeroMemoryAndMissingPCI(t *testing.T) {
	execSmi = func(ctx context.Context) ([][]string, error) {
		return [][]string{
			{"GPU-1", "A100", "0", "0", "0", "P0", "", "", "", "", "", "", "", "", "", "", "", ""},
		}, nil
	}
	t.Cleanup(func() { execSmi = runSmi })

	infos, err := queryNVML()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected single device")
	}
	if !infos[0].Partial || len(infos[0].Warnings) == 0 {
		t.Fatalf("expected warnings for zero memory and missing pci")
	}
}

func TestParseSmiRowsMissingFields(t *testing.T) {
	rows, err := parseSmiRows([][]string{{"GPU-1"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 || !rows[0].Partial || len(rows[0].Warnings) == 0 {
		t.Fatalf("expected partial data with warnings, got %+v", rows)
	}
}

func TestCollectPCIInfoMissing(t *testing.T) {
	if info, err := collectPCIInfo("", 0); err != nil || info.Address != "" {
		t.Fatalf("expected empty info without error")
	}
}

func TestNormalizePCIAddressFull(t *testing.T) {
	if got := normalizePCIAddress("0000:01:00.0"); got != "0000:01:00.0" {
		t.Fatalf("expected passthrough, got %s", got)
	}
}

func TestReadHexValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vendor")
	if err := os.WriteFile(path, []byte("0x10de\n"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	val, err := readHexValue(path)
	if err != nil || val != "0x10de" {
		t.Fatalf("unexpected readHexValue: %v %s", err, val)
	}
	if _, err := readHexValue(filepath.Join(dir, "missing")); err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestNVMLSearchPaths(t *testing.T) {
	t.Setenv("LD_LIBRARY_PATH", "/custom/lib:/usr/lib/x86_64-linux-gnu")
	paths := nvmlSearchPaths()
	if len(paths) == 0 {
		t.Fatalf("expected paths")
	}
	if paths[0] != "/custom/lib" {
		t.Fatalf("expected env path first, got %s", paths[0])
	}
	seen := map[string]struct{}{}
	for _, p := range paths {
		if _, ok := seen[p]; ok {
			t.Fatalf("duplicate path: %s", p)
		}
		seen[p] = struct{}{}
	}
}

func TestNVMLSearchPathsDedup(t *testing.T) {
	t.Setenv("LD_LIBRARY_PATH", "/lib:/lib")
	paths := nvmlSearchPaths()
	seen := map[string]struct{}{}
	for _, p := range paths {
		if _, ok := seen[p]; ok {
			t.Fatalf("duplicate path: %s", p)
		}
		seen[p] = struct{}{}
	}
}

func TestDescribeNVMLPresence(t *testing.T) {
	if describeNVMLPresence() == "" {
		t.Fatalf("expected description")
	}
}

func TestCollectPCIInfoFromSysfs(t *testing.T) {
	tmp := t.TempDir()
	pciDevicesRoot = tmp
	t.Cleanup(func() { pciDevicesRoot = sysfsPCIDevicesPath })

	addr := "0000:01:00.0"
	devDir := filepath.Join(tmp, addr)
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "vendor"), []byte("0x10de\n"), 0o644); err != nil {
		t.Fatalf("write vendor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "device"), []byte("0x1db6\n"), 0o644); err != nil {
		t.Fatalf("write device: %v", err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "class"), []byte("0x0300\n"), 0o644); err != nil {
		t.Fatalf("write class: %v", err)
	}

	info, err := collectPCIInfo(addr, 0)
	if err != nil {
		t.Fatalf("collectPCIInfo error: %v", err)
	}
	if info.Address != addr || info.Vendor != "0x10de" || info.Device != "0x1db6" || info.Class != "0x0300" {
		t.Fatalf("unexpected pci info: %+v", info)
	}
}

func TestRunSmiError(t *testing.T) {
	runSmiCommand = func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("not found")
	}
	t.Cleanup(func() {
		runSmiCommand = func(ctx context.Context, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, args[0], args[1:]...).Output()
		}
	})
	if _, err := runSmi(context.Background()); err == nil {
		t.Fatalf("expected runSmi to fail when nvidia-smi is absent")
	}
}

func TestRunSmiSuccess(t *testing.T) {
	runSmiCommand = func(context.Context, ...string) ([]byte, error) {
		return []byte("uuid,name,0,0,0,P0,0,0,0,0,0000:01:00.0,4,16,SERIAL,family,60,200,driver\n\n"), nil
	}
	t.Cleanup(func() {
		runSmiCommand = func(ctx context.Context, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, args[0], args[1:]...).Output()
		}
	})

	rows, err := runSmi(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 || len(rows[0]) < smiExpectedFields {
		t.Fatalf("expected one parsed row, got %#v", rows)
	}
}

func TestQueryNVMLError(t *testing.T) {
	execSmi = func(context.Context) ([][]string, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { execSmi = runSmi })
	if _, err := queryNVML(); err == nil {
		t.Fatalf("expected error from queryNVML")
	}
}

func TestNVMLSearchPathsSkipEmpty(t *testing.T) {
	t.Setenv("LD_LIBRARY_PATH", "::/tmp/lib")
	paths := nvmlSearchPaths()
	for _, p := range paths {
		if p == "" {
			t.Fatalf("unexpected empty path in nvmlSearchPaths")
		}
	}
}

func TestNVMLSearchPathsDedupDefault(t *testing.T) {
	t.Setenv("LD_LIBRARY_PATH", "/usr/local/nvidia/lib64")
	paths := nvmlSearchPaths()
	seen := map[string]struct{}{}
	for _, p := range paths {
		if _, ok := seen[p]; ok {
			t.Fatalf("duplicate path: %s", p)
		}
		seen[p] = struct{}{}
	}
}

func TestParseSmiOutputEmpty(t *testing.T) {
	rows := parseSmiOutput([]byte("\n"))
	if len(rows) != 0 {
		t.Fatalf("expected no rows, got %v", rows)
	}
}

func TestParseSmiOutputPadFields(t *testing.T) {
	rows := parseSmiOutput([]byte("uuid,name\n"))
	if len(rows) != 1 {
		t.Fatalf("expected one row")
	}
	if len(rows[0]) != smiExpectedFields {
		t.Fatalf("expected padded fields to %d, got %d", smiExpectedFields, len(rows[0]))
	}
}

func TestParseSmiRowsEmptyFields(t *testing.T) {
	rows, err := parseSmiRows([][]string{{"", ""}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 || !rows[0].Partial || len(rows[0].Warnings) == 0 {
		t.Fatalf("expected warnings for empty uuid/name, got %+v", rows)
	}
}

func TestRunSmiDefaultCommand(t *testing.T) {
	orig := runSmiCommand
	t.Cleanup(func() { runSmiCommand = orig })

	runSmiCommand = runSmiDefault
	if _, err := runSmiCommand(context.Background(), "true"); err != nil {
		t.Fatalf("expected runSmiDefault to execute simple command: %v", err)
	}
}
