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

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGeneratesCatalog(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.ids")
	output := filepath.Join(dir, "devices_gen.go")

	content := strings.Join([]string{
		"# comment",
		"    ", // whitespace only line
		"10de  NVIDIA Corporation",
		"\t",     // empty feature line
		"\t1db8", // missing name
		"\t1db6  NVIDIA GV100",
		"1abc  Other Vendor",
		"\t1fff  ShouldBeIgnored",
		"10de  NVIDIA Corporation",
		"\t1db7  NVIDIA GV100B",
	}, "\n")
	if err := os.WriteFile(input, []byte(content), 0o644); err != nil {
		t.Fatalf("write sample pci.ids: %v", err)
	}

	if err := run([]string{input, output}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	generated := string(data)
	if !strings.Contains(generated, "\"10de:1db6\"") || !strings.Contains(generated, "GV100") {
		t.Fatalf("generated file does not contain expected device: %s", generated)
	}
}

func TestRunReturnsErrorOnInvalidInput(t *testing.T) {
	if err := run([]string{"missing"}); err == nil {
		t.Fatalf("expected usage error for missing args")
	}

	if err := run([]string{"absent.ids", "out.go"}); err == nil {
		t.Fatalf("expected error when input file is missing")
	}
}

func TestRunReturnsErrorWhenNoDevicesFound(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.ids")
	if err := os.WriteFile(input, []byte("# empty"), 0o644); err != nil {
		t.Fatalf("write sample pci.ids: %v", err)
	}

	if err := run([]string{input, filepath.Join(dir, "devices.go")}); err == nil {
		t.Fatalf("expected error when no devices are found")
	}
}

func TestMainExitOnError(t *testing.T) {
	oldExit := exitFunc
	defer func() { exitFunc = oldExit }()
	exited := false
	exitFunc = func(int) { exited = true }

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"generate"}

	main()

	if !exited {
		t.Fatal("expected exitFunc to be invoked")
	}
}

func TestMainSuccess(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.ids")
	output := filepath.Join(dir, "devices.go")
	if err := os.WriteFile(input, []byte("10de  NVIDIA\n\t1db6  GPU"), 0o644); err != nil {
		t.Fatalf("write sample ids: %v", err)
	}

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"generate", input, output}

	oldExit := exitFunc
	defer func() { exitFunc = oldExit }()
	exitFunc = func(int) {
		t.Fatal("unexpected exit")
	}

	main()

	if _, err := os.Stat(output); err != nil {
		t.Fatalf("expected output file, got %v", err)
	}
}

func TestRunScannerError(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "invalid.ids")
	longLine := strings.Repeat("a", bufio.MaxScanTokenSize+10)
	content := strings.Join([]string{
		"10de  NVIDIA Corporation",
		"\t" + longLine,
	}, "\n")
	if err := os.WriteFile(input, []byte(content), 0o644); err != nil {
		t.Fatalf("write oversized ids: %v", err)
	}

	if err := run([]string{input, filepath.Join(dir, "out.go")}); err == nil {
		t.Fatal("expected scanner error")
	}
}

func TestRunWriteFileError(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.ids")
	if err := os.WriteFile(input, []byte("10de  NVIDIA\n\t1db6  GPU"), 0o644); err != nil {
		t.Fatalf("write sample ids: %v", err)
	}

	// use directory as output path to trigger write error
	if err := run([]string{input, dir}); err == nil {
		t.Fatal("expected error when writing to directory")
	}
}
