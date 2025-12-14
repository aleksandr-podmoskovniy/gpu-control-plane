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

package indexer

import "testing"

func TestMapIndexer(t *testing.T) {
	idx := NewMapIndexer()
	idx.AddPair("a", "b")

	if got := idx.Rename("a"); got != "b" {
		t.Fatalf("Rename(a)=%q, want %q", got, "b")
	}
	if got := idx.Rename("missing"); got != "missing" {
		t.Fatalf("Rename(missing)=%q, want %q", got, "missing")
	}

	if got := idx.Restore("b"); got != "a" {
		t.Fatalf("Restore(b)=%q, want %q", got, "a")
	}
	if got := idx.Restore("missing"); got != "missing" {
		t.Fatalf("Restore(missing)=%q, want %q", got, "missing")
	}

	if !idx.IsOriginal("a") || idx.IsOriginal("missing") {
		t.Fatalf("unexpected IsOriginal results")
	}
	if !idx.IsRenamed("b") || idx.IsRenamed("missing") {
		t.Fatalf("unexpected IsRenamed results")
	}
}
