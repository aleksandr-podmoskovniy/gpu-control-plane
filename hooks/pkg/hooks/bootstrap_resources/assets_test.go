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

package bootstrap_resources

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestEmbeddedCRDsUpToDate ensures that CRDs shipped with the hooks binary
// stay in sync with the canonical manifests living in the root crds/ directory.
func TestEmbeddedCRDsUpToDate(t *testing.T) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	assetsDir := filepath.Join(wd, "assets", "crds")
	rootCRDsDir := filepath.Clean(filepath.Join(wd, "../../../..", "crds"))

	assetEntries, err := os.ReadDir(assetsDir)
	if err != nil {
		t.Fatalf("read assets dir: %v", err)
	}
	rootEntries, err := os.ReadDir(rootCRDsDir)
	if err != nil {
		t.Fatalf("read root CRDs dir: %v", err)
	}

	wantNames := collectCRDNames(rootEntries)
	gotNames := collectCRDNames(assetEntries)

	if len(wantNames) != len(gotNames) {
		t.Fatalf("CRD asset mismatch: expected %d files, got %d (%v vs %v)", len(wantNames), len(gotNames), wantNames, gotNames)
	}

	for _, name := range wantNames {
		if !slices.Contains(gotNames, name) {
			t.Fatalf("CRD asset %q missing in hooks assets directory", name)
		}

		rootPayload, err := os.ReadFile(filepath.Join(rootCRDsDir, name))
		if err != nil {
			t.Fatalf("read root CRD %q: %v", name, err)
		}
		assetPayload, err := os.ReadFile(filepath.Join(assetsDir, name))
		if err != nil {
			t.Fatalf("read asset CRD %q: %v", name, err)
		}

		if string(rootPayload) != string(assetPayload) {
			t.Fatalf("CRD asset %q is out of sync with root manifest", name)
		}
	}
}

func collectCRDNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "doc-") {
			continue
		}
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
