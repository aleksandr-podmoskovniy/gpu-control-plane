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

package rewriter

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRestoreCRD_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("errors on malformed CRD name", func(t *testing.T) {
		_, err := RestoreCRD(rules, []byte(`{"metadata":{"name":"someresources"}}`))
		require.Error(t, err)
	})

	t.Run("skips CRD with original group", func(t *testing.T) {
		_, err := RestoreCRD(rules, []byte(`{"metadata":{"name":"someresources.original.group.io"}}`))
		require.ErrorIs(t, err, SkipItem)
	})

	t.Run("ignores CRD from unknown group", func(t *testing.T) {
		res, err := RestoreCRD(rules, []byte(`{"metadata":{"name":"prefixedsomeresources.unknown.group.io"}}`))
		require.NoError(t, err)
		require.Nil(t, res)
	})

	t.Run("ignores CRD without resource rules", func(t *testing.T) {
		res, err := RestoreCRD(rules, []byte(`{"metadata":{"name":"prefixedunknownresources.prefixed.resources.group.io"}}`))
		require.NoError(t, err)
		require.Nil(t, res)
	})

	t.Run("restores known CRD", func(t *testing.T) {
		obj := []byte(`{
  "metadata":{"name":"prefixedsomeresources.prefixed.resources.group.io"},
  "spec":{
    "group":"prefixed.resources.group.io",
    "names":{
      "categories":["prefixed"],
      "kind":"PrefixedSomeResource",
      "listKind":"PrefixedSomeResourceList",
      "plural":"prefixedsomeresources",
      "singular":"prefixedsomeresource",
      "shortNames":["psr","psrs"]
    }
  }
}`)
		res, err := RestoreCRD(rules, obj)
		require.NoError(t, err)
		require.Equal(t, "someresources.original.group.io", gjson.GetBytes(res, "metadata.name").String())
		require.Equal(t, "original.group.io", gjson.GetBytes(res, "spec.group").String())
		require.Equal(t, "SomeResource", gjson.GetBytes(res, "spec.names.kind").String())
		require.Equal(t, "SomeResourceList", gjson.GetBytes(res, "spec.names.listKind").String())
		require.Equal(t, "someresources", gjson.GetBytes(res, "spec.names.plural").String())
		require.Equal(t, "someresource", gjson.GetBytes(res, "spec.names.singular").String())
		require.Equal(t, `["sr","srs"]`, gjson.GetBytes(res, "spec.names.shortNames").Raw)
		require.Equal(t, `["all"]`, gjson.GetBytes(res, "spec.names.categories").Raw)
	})

	t.Run("returns errors from sjson setters", func(t *testing.T) {
		base := []byte(`{
  "metadata":{"name":"prefixedsomeresources.prefixed.resources.group.io"},
  "spec":{
    "group":"prefixed.resources.group.io",
    "names":{
      "categories":["prefixed"],
      "kind":"PrefixedSomeResource",
      "listKind":"PrefixedSomeResourceList",
      "plural":"prefixedsomeresources",
      "singular":"prefixedsomeresource",
      "shortNames":["psr","psrs"]
    }
  }
}`)

		failPaths := []string{
			"metadata.name",
			"spec.group",
			"categories",
			"kind",
			"listKind",
			"plural",
			"singular",
			"shortNames",
		}
		for _, failPath := range failPaths {
			t.Run(failPath, func(t *testing.T) {
				failSjsonSetBytesOnPath(t, failPath)
				_, err := RestoreCRD(rules, base)
				require.Error(t, err)
			})
		}

		t.Run("spec.names", func(t *testing.T) {
			failSjsonSetRawBytesOnPath(t, "spec.names")
			_, err := RestoreCRD(rules, base)
			require.Error(t, err)
		})
	})
}

func TestRenameCRD_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("errors on malformed CRD name", func(t *testing.T) {
		_, err := RenameCRD(rules, []byte(`{"metadata":{"name":"someresources"}}`))
		require.Error(t, err)
	})

	t.Run("ignores CRD without rules", func(t *testing.T) {
		res, err := RenameCRD(rules, []byte(`{"metadata":{"name":"unknownresources.original.group.io"}}`))
		require.NoError(t, err)
		require.Nil(t, res)
	})

	t.Run("errors on setting metadata.name", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "metadata.name")
		_, err := RenameCRD(rules, []byte(`{"metadata":{"name":"someresources.original.group.io"},"spec":{"group":"original.group.io","names":{"kind":"SomeResource"}}}`))
		require.Error(t, err)
	})

	t.Run("errors from renameCRDSpec", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "group")
		_, err := RenameCRD(rules, []byte(`{"metadata":{"name":"someresources.original.group.io"},"spec":{"group":"original.group.io","names":{"kind":"SomeResource"}}}`))
		require.Error(t, err)
	})

	t.Run("errors on setting spec", func(t *testing.T) {
		failSjsonSetRawBytesOnPath(t, "spec")
		_, err := RenameCRD(rules, []byte(`{"metadata":{"name":"someresources.original.group.io"},"spec":{"group":"original.group.io","names":{"kind":"SomeResource"}}}`))
		require.Error(t, err)
	})
}

func TestRenameCRDSpec_ErrorBranches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules
	_, resRule := rules.ResourceRules("original.group.io", "someresources")
	require.NotNil(t, resRule)

	spec := []byte(`{"group":"original.group.io","names":{"categories":["all"],"kind":"SomeResource","listKind":"SomeResourceList","plural":"someresources","singular":"someresource","shortNames":["sr","srs"]}}`)

	t.Run("errors on group transform", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "group")
		_, err := renameCRDSpec(rules, resRule, spec)
		require.Error(t, err)
	})

	failPaths := []string{"categories", "kind", "listKind", "plural", "singular", "shortNames"}
	for _, failPath := range failPaths {
		t.Run("errors on "+failPath, func(t *testing.T) {
			failSjsonSetBytesOnPath(t, failPath)
			_, err := renameCRDSpec(rules, resRule, spec)
			require.Error(t, err)
		})
	}

	t.Run("errors on names set", func(t *testing.T) {
		failSjsonSetRawBytesOnPath(t, "names")
		_, err := renameCRDSpec(rules, resRule, spec)
		require.Error(t, err)
	})
}

func TestRenameCRDPatch_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules
	_, resRule := rules.ResourceRules("original.group.io", "someresources")
	require.NotNil(t, resRule)

	t.Run("returns error from RenameMetadataPatch", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "value")
		_, err := RenameCRDPatch(rules, resRule, []byte(`[{"op":"replace","path":"/metadata/labels","value":{"labelgroup.io":"v"}}]`))
		require.Error(t, err)
	})

	t.Run("does not rename when patch has no /spec", func(t *testing.T) {
		patch := []byte(`[{"op":"replace","path":"/metadata/labels","value":{"labelgroup.io":"v"}}]`)
		res, err := RenameCRDPatch(rules, resRule, patch)
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(res, `0.value.replacedlabelgroup\.io`).String())
	})

	t.Run("swallows errors from renameCRDSpec (current behavior)", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "group")
		patch := []byte(`[{"op":"replace","path":"/spec","value":{"group":"original.group.io","names":{"kind":"SomeResource"}}}]`)
		res, err := RenameCRDPatch(rules, resRule, patch)
		require.NoError(t, err)
		require.Nil(t, res)
	})

	t.Run("swallows errors from setting patch value (current behavior)", func(t *testing.T) {
		failSjsonSetRawBytesOnPath(t, "value")
		patch := []byte(`[{"op":"replace","path":"/spec","value":{"group":"original.group.io","names":{"kind":"SomeResource"}}}]`)
		res, err := RenameCRDPatch(rules, resRule, patch)
		require.NoError(t, err)
		require.Nil(t, res)
	})
}
