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

func TestResourceRuleRewriters_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("rename does nothing for unknown group", func(t *testing.T) {
		obj := []byte(`{"apiGroups":["unknown.group.io"],"resources":["someresources"]}`)
		res, err := RenameResourceRule(rules, obj)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("rename rewrites resources for wildcard group", func(t *testing.T) {
		obj := []byte(`{"apiGroups":["*"],"resources":["someresources"]}`)
		res, err := RenameResourceRule(rules, obj)
		require.NoError(t, err)
		require.Equal(t, "prefixedsomeresources", gjson.GetBytes(res, "resources.0").String())
	})

	t.Run("rename keeps resources without rules", func(t *testing.T) {
		obj := []byte(`{"apiGroups":["original.group.io"],"resources":["unknownresources"]}`)
		res, err := RenameResourceRule(rules, obj)
		require.NoError(t, err)
		require.Equal(t, "unknownresources", gjson.GetBytes(res, "resources.0").String())
	})

	t.Run("rename keeps wildcard and empty resources", func(t *testing.T) {
		obj := []byte(`{"apiGroups":["original.group.io"],"resources":["*",""]}`)
		res, err := RenameResourceRule(rules, obj)
		require.NoError(t, err)
		require.Equal(t, "*", gjson.GetBytes(res, "resources.0").String())
		require.Equal(t, "", gjson.GetBytes(res, "resources.1").String())
	})

	t.Run("restore does nothing for unknown group", func(t *testing.T) {
		obj := []byte(`{"apiGroups":["unknown.group.io"],"resources":["prefixedsomeresources"]}`)
		res, err := RestoreResourceRule(rules, obj)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("restore rewrites resources for wildcard group", func(t *testing.T) {
		obj := []byte(`{"apiGroups":["*"],"resources":["prefixedsomeresources/status"]}`)
		res, err := RestoreResourceRule(rules, obj)
		require.NoError(t, err)
		require.Equal(t, "someresources/status", gjson.GetBytes(res, "resources.0").String())
	})

	t.Run("restore keeps resources without rules", func(t *testing.T) {
		obj := []byte(`{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedunknownresources"]}`)
		res, err := RestoreResourceRule(rules, obj)
		require.NoError(t, err)
		require.Equal(t, "prefixedunknownresources", gjson.GetBytes(res, "resources.0").String())
	})

	t.Run("restore keeps wildcard and empty resources", func(t *testing.T) {
		obj := []byte(`{"apiGroups":["prefixed.resources.group.io"],"resources":["*",""]}`)
		res, err := RestoreResourceRule(rules, obj)
		require.NoError(t, err)
		require.Equal(t, "*", gjson.GetBytes(res, "resources.0").String())
		require.Equal(t, "", gjson.GetBytes(res, "resources.1").String())
	})

	t.Run("rename returns error from TransformArrayOfStrings", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "apiGroups")
		_, err := RenameResourceRule(rules, []byte(`{"apiGroups":["original.group.io"],"resources":["someresources"]}`))
		require.Error(t, err)
	})

	t.Run("restore returns error from TransformArrayOfStrings", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "apiGroups")
		_, err := RestoreResourceRule(rules, []byte(`{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"]}`))
		require.Error(t, err)
	})
}
