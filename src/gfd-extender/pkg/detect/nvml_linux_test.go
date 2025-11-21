//go:build linux && cgo

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

import "testing"

func TestNvmlSearchPathsPrefersRuntimeLibs(t *testing.T) {
	paths := nvmlSearchPaths()
	if len(paths) < 2 {
		t.Fatalf("unexpected paths: %v", paths)
	}
	if paths[0] != "/usr/local/nvidia/lib64" || paths[1] != "/usr/local/nvidia/lib" {
		t.Fatalf("unexpected preferred paths: %v", paths[:2])
	}
}
