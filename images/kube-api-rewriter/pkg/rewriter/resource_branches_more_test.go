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

func TestRewriteCustomResourceOrList_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("unknown kind passthrough", func(t *testing.T) {
		obj := []byte(`{"kind":"UnknownKind","apiVersion":"v1"}`)
		res, err := RewriteCustomResourceOrList(rules, obj, Rename)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("rename list and items", func(t *testing.T) {
		obj := []byte(`{"kind":"SomeResourceList","apiVersion":"original.group.io/v1","items":[{"kind":"SomeResource","apiVersion":"original.group.io/v1"}]}`)
		res, err := RewriteCustomResourceOrList(rules, obj, Rename)
		require.NoError(t, err)
		require.Equal(t, "PrefixedSomeResourceList", gjson.GetBytes(res, "kind").String())
		require.Equal(t, "prefixed.resources.group.io/v1", gjson.GetBytes(res, "apiVersion").String())
		require.Equal(t, "PrefixedSomeResource", gjson.GetBytes(res, "items.0.kind").String())
		require.Equal(t, "prefixed.resources.group.io/v1", gjson.GetBytes(res, "items.0.apiVersion").String())
	})

	t.Run("restore list and items", func(t *testing.T) {
		obj := []byte(`{"kind":"PrefixedSomeResourceList","apiVersion":"prefixed.resources.group.io/v1","items":[{"kind":"PrefixedSomeResource","apiVersion":"prefixed.resources.group.io/v1"}]}`)
		res, err := RewriteCustomResourceOrList(rules, obj, Restore)
		require.NoError(t, err)
		require.Equal(t, "SomeResourceList", gjson.GetBytes(res, "kind").String())
		require.Equal(t, "original.group.io/v1", gjson.GetBytes(res, "apiVersion").String())
		require.Equal(t, "SomeResource", gjson.GetBytes(res, "items.0.kind").String())
		require.Equal(t, "original.group.io/v1", gjson.GetBytes(res, "items.0.apiVersion").String())
	})

	t.Run("rename single resource and managedFields", func(t *testing.T) {
		obj := []byte(`{
  "kind":"SomeResource",
  "apiVersion":"original.group.io/v1",
  "metadata":{"managedFields":[{"apiVersion":"original.group.io/v1"}]}
}`)
		res, err := RewriteCustomResourceOrList(rules, obj, Rename)
		require.NoError(t, err)
		require.Equal(t, "PrefixedSomeResource", gjson.GetBytes(res, "kind").String())
		require.Equal(t, "prefixed.resources.group.io/v1", gjson.GetBytes(res, "apiVersion").String())
		require.Equal(t, "prefixed.resources.group.io/v1", gjson.GetBytes(res, "metadata.managedFields.0.apiVersion").String())
	})

	t.Run("restore single resource", func(t *testing.T) {
		obj := []byte(`{"kind":"PrefixedSomeResource","apiVersion":"prefixed.resources.group.io/v1"}`)
		res, err := RewriteCustomResourceOrList(rules, obj, Restore)
		require.NoError(t, err)
		require.Equal(t, "SomeResource", gjson.GetBytes(res, "kind").String())
		require.Equal(t, "original.group.io/v1", gjson.GetBytes(res, "apiVersion").String())
	})
}

func TestResourceListAndKindRewriters_ErrorBranches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("RenameResourcesList errors on RenameAPIVersionAndKind", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "apiVersion")
		_, err := RenameResourcesList(rules, []byte(`{"kind":"SomeResourceList","apiVersion":"original.group.io/v1","items":[]}`))
		require.Error(t, err)
	})

	t.Run("RestoreResourcesList errors on RestoreAPIVersionAndKind", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "apiVersion")
		_, err := RestoreResourcesList(rules, []byte(`{"kind":"PrefixedSomeResourceList","apiVersion":"prefixed.resources.group.io/v1","items":[]}`))
		require.Error(t, err)
	})

	t.Run("RenameResource errors on RenameAPIVersionAndKind", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "apiVersion")
		_, err := RenameResource(rules, []byte(`{"kind":"SomeResource","apiVersion":"original.group.io/v1"}`))
		require.Error(t, err)
	})

	t.Run("RenameAPIVersionAndKind errors on apiVersion set", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "apiVersion")
		_, err := RenameAPIVersionAndKind(rules, []byte(`{"kind":"SomeResource","apiVersion":"original.group.io/v1"}`))
		require.Error(t, err)
	})
}
