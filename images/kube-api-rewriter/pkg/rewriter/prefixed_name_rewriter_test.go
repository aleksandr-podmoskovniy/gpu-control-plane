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

package rewriter

import "testing"

func sampleRules() MetadataReplace {
	return MetadataReplace{
		Names: []MetadataReplaceRule{
			{Original: "gpu.deckhouse.io/managed-by", Renamed: "internal.gpu.deckhouse.io/managed-by"},
			{Original: "gpu.deckhouse.io/feature", OriginalValue: "enabled", Renamed: "internal.gpu.deckhouse.io/feature", RenamedValue: "true"},
		},
		Prefixes: []MetadataReplaceRule{
			{Original: "gpu.deckhouse.io", Renamed: "internal.gpu.deckhouse.io"},
		},
	}
}

func TestPrefixedNameRewriterRenameRestoreExact(t *testing.T) {
	rewrite := NewPrefixedNameRewriter(sampleRules())

	name, value := rewrite.Rename("gpu.deckhouse.io/feature", "enabled")
	if name != "internal.gpu.deckhouse.io/feature" || value != "true" {
		t.Fatalf("unexpected rename result: %s=%s", name, value)
	}

	origName, origValue := rewrite.Restore(name, value)
	if origName != "gpu.deckhouse.io/feature" || origValue != "enabled" {
		t.Fatalf("unexpected restore result: %s=%s", origName, origValue)
	}

	// When original label is passed to restore, it should be preserved
	preservedName, preservedValue := rewrite.Restore(origName, origValue)
	if preservedName != PreservedPrefix+"gpu.deckhouse.io/feature" || preservedValue != "enabled" {
		t.Fatalf("expected preserved label on second restore, got %s=%s", preservedName, preservedValue)
	}

	unwrappedName, unwrappedValue := rewrite.Rename(preservedName, preservedValue)
	if unwrappedName != "gpu.deckhouse.io/feature" || unwrappedValue != "enabled" {
		t.Fatalf("expected preserved label to unwrap, got %s=%s", unwrappedName, unwrappedValue)
	}
}

func TestPrefixedNameRewriterSliceAndMap(t *testing.T) {
	rewrite := NewPrefixedNameRewriter(sampleRules())

	slice := []string{"gpu.deckhouse.io/custom", "other"}
	renamedSlice := rewrite.RenameSlice(slice)
	if renamedSlice[0] != "internal.gpu.deckhouse.io/custom" || renamedSlice[1] != "other" {
		t.Fatalf("unexpected rename slice result: %+v", renamedSlice)
	}

	restoredSlice := rewrite.RestoreSlice(renamedSlice)
	if restoredSlice[0] != "gpu.deckhouse.io/custom" {
		t.Fatalf("unexpected restore slice result: %+v", restoredSlice)
	}

	m := map[string]string{"gpu.deckhouse.io/managed-by": "controller"}
	renamedMap := rewrite.RenameMap(m)
	if renamedMap["internal.gpu.deckhouse.io/managed-by"] != "controller" {
		t.Fatalf("unexpected rename map: %+v", renamedMap)
	}

	restoredMap := rewrite.RestoreMap(renamedMap)
	if restoredMap["gpu.deckhouse.io/managed-by"] != "controller" {
		t.Fatalf("unexpected restored map: %+v", restoredMap)
	}
}

func TestPrefixedNameRewriterRewriteNameValues(t *testing.T) {
	rewrite := NewPrefixedNameRewriter(sampleRules())

	name, values := rewrite.RewriteNameValues("gpu.deckhouse.io/feature", []string{"enabled", "other"}, Rename)
	if name != "internal.gpu.deckhouse.io/feature" {
		t.Fatalf("unexpected rewritten name: %s", name)
	}
	if values[0] != "true" || values[1] != "other" {
		t.Fatalf("unexpected values: %+v", values)
	}

	restoredName, restoredValues := rewrite.RewriteNameValues(name, values, Restore)
	if restoredName != "gpu.deckhouse.io/feature" || restoredValues[0] != "enabled" {
		t.Fatalf("unexpected restored outcome: %s=%v", restoredName, restoredValues)
	}
}

func TestPrefixedNameRewriterPreservedDetection(t *testing.T) {
	rewrite := NewPrefixedNameRewriter(sampleRules())

	if !rewrite.isOriginal("gpu.deckhouse.io/custom", "") {
		t.Fatalf("expected label to be treated original")
	}

	wrapped := rewrite.preserveName("gpu.deckhouse.io/custom")
	name, value := rewrite.Rename(wrapped, "value")
	if name != "gpu.deckhouse.io/custom" || value != "value" {
		t.Fatalf("preserved label should unwrap, got %s=%s", name, value)
	}
}
