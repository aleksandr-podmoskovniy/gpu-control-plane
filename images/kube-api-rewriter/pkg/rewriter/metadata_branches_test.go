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

func TestRewriteMetadata_ErrorBranches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	rules.Finalizers.Prefixes = append(rules.Finalizers.Prefixes, MetadataReplaceRule{
		Original: "group.io",
		Renamed:  "replaced.group.io",
	})
	rules.Init()

	obj := []byte(`{
  "labels":{"labelgroup.io":"v"},
  "annotations":{"annogroup.io":"a"},
  "finalizers":["group.io/protection"],
  "ownerReferences":[{"apiVersion":"original.group.io/v1","kind":"SomeResource","name":"o"}]
}`)

	t.Run("labels", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "labels")
		_, err := RewriteMetadata(rules, obj, Rename)
		require.Error(t, err)
	})

	t.Run("annotations", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "annotations")
		_, err := RewriteMetadata(rules, obj, Rename)
		require.Error(t, err)
	})

	t.Run("finalizers", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "finalizers")
		_, err := RewriteMetadata(rules, obj, Rename)
		require.Error(t, err)
	})

	t.Run("ownerReferences", func(t *testing.T) {
		failSjsonSetRawBytesOnPath(t, "ownerReferences")
		_, err := RewriteMetadata(rules, obj, Rename)
		require.Error(t, err)
	})
}

func TestRewriteMetadata_AllFields(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	rules.Finalizers.Prefixes = append(rules.Finalizers.Prefixes, MetadataReplaceRule{
		Original: "group.io",
		Renamed:  "replaced.group.io",
	})
	rules.Init()

	obj := []byte(`{
  "labels":{"labelgroup.io":"v"},
  "annotations":{"annogroup.io":"a"},
  "finalizers":["group.io/protection"],
  "ownerReferences":[{"apiVersion":"original.group.io/v1","kind":"SomeResource","name":"o"}]
}`)

	res, err := RewriteMetadata(rules, obj, Rename)
	require.NoError(t, err)
	require.Equal(t, "v", gjson.GetBytes(res, `labels.replacedlabelgroup\.io`).String())
	require.Equal(t, "a", gjson.GetBytes(res, `annotations.replacedanno\.io`).String())
	require.Equal(t, `["replaced.group.io/protection"]`, gjson.GetBytes(res, "finalizers").Raw)
	require.Equal(t, "prefixed.resources.group.io/v1", gjson.GetBytes(res, "ownerReferences.0.apiVersion").String())
	require.Equal(t, "PrefixedSomeResource", gjson.GetBytes(res, "ownerReferences.0.kind").String())
}
