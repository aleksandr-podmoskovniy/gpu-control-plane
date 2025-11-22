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

//go:build linux

package detect

import (
	"context"
	"errors"
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
	execSmi = func(ctx context.Context) ([][]string, error) {
		// second device will produce many missing fields
		return [][]string{
			{"GPU-1", "A100", "1024", "512", "512", "P0", "1100", "900", "10", "20", "0000:01:00.0", "4", "16", "SERIAL", "ampere", "60", "200", "535.104.06"},
			{"GPU-2"},
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
	if len(infos[1].Warnings) == 0 {
		t.Fatalf("expected warnings for second device")
	}
	if len(infos[1].Warnings) > maxWarningsPerGPU+1 { // +1 for truncated marker
		t.Fatalf("warnings should be truncated, got %d", len(infos[1].Warnings))
	}
}
