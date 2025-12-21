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

package state

import (
	"strings"
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestParseHardwareDefaultsSetsCapableWhenProfilesWithoutTypes(t *testing.T) {
	labels := map[string]string{
		// Unsupported metric -> profiles supported, but types remain empty.
		"nvidia.com/mig-1g.10gb.unknown": "1",
	}
	defaults := parseHardwareDefaults(labels)
	if !defaults.MIG.Capable {
		t.Fatalf("expected MIG to become capable when profiles are present")
	}
	if len(defaults.MIG.ProfilesSupported) != 1 || defaults.MIG.ProfilesSupported[0] != "1g.10gb" {
		t.Fatalf("unexpected profilesSupported: %+v", defaults.MIG.ProfilesSupported)
	}
	if len(defaults.MIG.Types) != 0 {
		t.Fatalf("expected no types for unknown metric, got %+v", defaults.MIG.Types)
	}
}

func TestParseMIGConfigCollectsMetrics(t *testing.T) {
	labels := map[string]string{
		gfdMigCapableLabel:                       "true",
		gfdMigStrategyLabel:                      "mixed",
		"nvidia.com/mig-1g.10gb.count":           "2",
		"nvidia.com/mig-1g.10gb.engines.copy":    "4",
		"nvidia.com/mig-1g.10gb.engines.encoder": "1",
		"nvidia.com/mig-1g.10gb.engines.decoder": "1",
		"nvidia.com/mig-1g.10gb.engines.ofa":     "1",
		"nvidia.com/mig-1g.10gb.memory":          "10240",
		"nvidia.com/mig-1g.10gb.multiprocessors": "14",
	}

	cfg := parseMIGConfig(labels)
	if !cfg.Capable || cfg.Strategy != v1alpha1.GPUMIGStrategyMixed {
		t.Fatalf("unexpected MIG config: %+v", cfg)
	}
	if len(cfg.ProfilesSupported) != 1 || cfg.ProfilesSupported[0] != "1g.10gb" {
		t.Fatalf("unexpected profiles: %+v", cfg.ProfilesSupported)
	}
	if len(cfg.Types) != 1 {
		t.Fatalf("expected single MIG type, got %+v", cfg.Types)
	}
	migType := cfg.Types[0]
	if migType.Count != 2 || migType.Name != "1g.10gb" {
		t.Fatalf("unexpected MIG type capacity: %+v", migType)
	}
}

func TestParseMemoryMiBFallbackDigits(t *testing.T) {
	if parseMemoryMiB("512foobar") != 512 {
		t.Fatalf("expected parser to extract leading digits")
	}
	if parseMemoryMiB("") != 0 {
		t.Fatalf("expected empty value to return 0")
	}
}

func TestParseDriverInfoRuntimeFallback(t *testing.T) {
	info := parseDriverInfo(map[string]string{
		gfdDriverVersionLabel:      "535.80.10",
		gfdCudaRuntimeVersionLabel: "12.4",
		deckhouseToolkitReadyLabel: "true",
		deckhouseToolkitInstalled:  "false",
	})
	if info.CUDAVersion != "12.4" {
		t.Fatalf("expected runtime version fallback, got %s", info.CUDAVersion)
	}
	if !info.ToolkitInstalled || !info.ToolkitReady {
		t.Fatalf("expected toolkit installed to be forced when ready, got %+v", info)
	}
}

func TestParseMemoryMiBVariants(t *testing.T) {
	if got := parseMemoryMiB("40960 MiB"); got != 40960 {
		t.Fatalf("expected 40960 MiB, got %d", got)
	}
	if got := parseMemoryMiB("40 GiB"); got != 40960 {
		t.Fatalf("expected GiB to be converted to MiB, got %d", got)
	}
	if got := parseMemoryMiB("0.5 TiB"); got != 524288 {
		t.Fatalf("expected TiB conversion, got %d", got)
	}
	if got := parseMemoryMiB("unknown"); got != 0 {
		t.Fatalf("invalid memory should return 0, got %d", got)
	}
}

func TestParseMemoryMiBHandlesErrRange(t *testing.T) {
	big := strings.Repeat("9", 400) + " MiB"
	if parseMemoryMiB(big) != 0 {
		t.Fatalf("expected overflow to return 0")
	}
}

func TestParseInt32Variants(t *testing.T) {
	if got := parseInt32("42"); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if got := parseInt32("42 MHz"); got != 42 {
		t.Fatalf("expected to parse leading digits, got %d", got)
	}
	if got := parseInt32("not-a-number"); got != 0 {
		t.Fatalf("expected parse failure to return 0, got %d", got)
	}
}

func TestParseInt32HandlesErrRange(t *testing.T) {
	big := strings.Repeat("9", 40)
	if parseInt32(big) != 0 {
		t.Fatalf("expected overflow to return 0")
	}
}

func TestParseInt32HandlesLeadingZeroes(t *testing.T) {
	if got := parseInt32("007"); got != 7 {
		t.Fatalf("expected leading zeroes to be parsed, got %d", got)
	}
}

func TestExtractLeadingDigits(t *testing.T) {
	if got := extractLeadingDigits("123abc"); got != "123" {
		t.Fatalf("expected leading digits extracted, got %q", got)
	}
	if got := extractLeadingDigits("abc123"); got != "" {
		t.Fatalf("expected empty string when no leading digits, got %q", got)
	}
}

func TestParseMIGConfigVariants(t *testing.T) {
	cfg := parseMIGConfig(map[string]string{
		gfdMigCapableLabel:             "true",
		"nvidia.com/mig.strategy":      "mixed",
		"nvidia.com/mig-1g.10gb.count": "2",
	})
	if !cfg.Capable {
		t.Fatal("expected MIG capable true")
	}
	if cfg.Strategy != v1alpha1.GPUMIGStrategyMixed {
		t.Fatalf("unexpected strategy: %s", cfg.Strategy)
	}
	if len(cfg.Types) != 1 || cfg.Types[0].Name != "1g.10gb" || cfg.Types[0].Count != 2 {
		t.Fatalf("unexpected MIG types: %+v", cfg.Types)
	}

	cfg = parseMIGConfig(map[string]string{
		gfdMigAltCapableLabel: "false",
	})
	if cfg.Capable {
		t.Fatal("expected MIG capable false from alt label")
	}
	if len(cfg.Types) != 0 {
		t.Fatalf("unexpected MIG types when not capable: %+v", cfg.Types)
	}
}

func TestParseMIGConfigIgnoresMalformedKeys(t *testing.T) {
	labels := map[string]string{
		"nvidia.com/mig-foo":           "1",
		"nvidia.com/mig-1g":            "",
		"nvidia.com/mig-1g.profile":    "",
		"nvidia.com/mig-1g.5gb.":       "",
		"nvidia.com/mig-3g.40gb.count": "",
	}
	cfg := parseMIGConfig(labels)
	if len(cfg.Types) != 0 {
		t.Fatalf("expected malformed labels to be ignored, got %+v", cfg.Types)
	}
}

func TestParseMIGConfigUnknownStrategy(t *testing.T) {
	cfg := parseMIGConfig(map[string]string{
		"nvidia.com/mig.strategy": "unsupported",
	})
	if cfg.Strategy != v1alpha1.GPUMIGStrategyNone {
		t.Fatalf("expected strategy fallback to none, got %s", cfg.Strategy)
	}
}

func TestParseMIGConfigSortsMultipleTypes(t *testing.T) {
	labels := map[string]string{
		"nvidia.com/mig.capable":       "true",
		"nvidia.com/mig-1g.10gb.count": "2",
		"nvidia.com/mig-2g.20gb.count": "1",
	}
	cfg := parseMIGConfig(labels)
	if len(cfg.Types) != 2 {
		t.Fatalf("expected two types, got %+v", cfg.Types)
	}
	if cfg.Types[0].Name != "1g.10gb" || cfg.Types[1].Name != "2g.20gb" {
		t.Fatalf("expected sorted types, got %+v", cfg.Types)
	}
}

func TestParseMIGConfigAlternativeLabels(t *testing.T) {
	cfg := parseMIGConfig(map[string]string{
		gfdMigAltCapableLabel:              "true",
		gfdMigAltStrategy:                  "single",
		"nvidia.com/mig-2g.20gb.count":     "1",
		"nvidia.com/mig-2g.20gb.ready":     "1",
		"nvidia.com/mig-2g.20gb.available": "1",
	})
	if !cfg.Capable || cfg.Strategy != v1alpha1.GPUMIGStrategySingle {
		t.Fatalf("expected capability and strategy from alternative labels, got %+v", cfg)
	}
	if len(cfg.Types) != 1 || cfg.Types[0].Name != "2g.20gb" {
		t.Fatalf("expected alt label to produce type, got %+v", cfg.Types)
	}
}
