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

func TestSliceBytesBuilder_WriteBranches(t *testing.T) {
	b := newSliceBytesBuilder()

	b.Write(nil)
	b.Write([]byte{})
	b.Write([]byte(`{"a":1}`))
	b.Write([]byte(`{"b":2}`))

	require.Equal(t, `[{"a":1},{"b":2}]`, string(b.Complete().Bytes()))
}

func TestRewriteAPIGroupList_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("restores renamed group and skips original duplicate", func(t *testing.T) {
		obj := []byte(`{
  "groups":[
    {"name":"original.group.io","versions":[{"groupVersion":"original.group.io/v1","version":"v1"}],"preferredVersion":{"groupVersion":"original.group.io/v1","version":"v1"}},
    {"name":"prefixed.resources.group.io","versions":[{"groupVersion":"prefixed.resources.group.io/v1","version":"v1"}],"preferredVersion":{"groupVersion":"prefixed.resources.group.io/v1","version":"v1"}}
  ]
}`)
		res, err := RewriteAPIGroupList(rules, obj)
		require.NoError(t, err)
		require.Equal(t, "original.group.io", gjson.GetBytes(res, "groups.0.name").String())
		require.Equal(t, "original.group.io/v1", gjson.GetBytes(res, "groups.0.versions.0.groupVersion").String())
		require.Equal(t, "original.group.io/v1", gjson.GetBytes(res, "groups.0.preferredVersion.groupVersion").String())
	})

	t.Run("errors on name transform", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "name")
		_, err := RewriteAPIGroupList(rules, []byte(`{"groups":[{"name":"prefixed.resources.group.io"}]}`))
		require.Error(t, err)
	})

	t.Run("errors on preferredVersion transform", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "preferredVersion.groupVersion")
		_, err := RewriteAPIGroupList(rules, []byte(`{"groups":[{"name":"prefixed.resources.group.io","preferredVersion":{"groupVersion":"prefixed.resources.group.io/v1"}}]}`))
		require.Error(t, err)
	})
}

func TestRewriteAPIGroup_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("passthrough for non-renamed group", func(t *testing.T) {
		obj := []byte(`{"name":"original.group.io"}`)
		res, err := RewriteAPIGroup(rules, obj)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("errors on name set", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "name")
		_, err := RewriteAPIGroup(rules, []byte(`{"name":"prefixed.resources.group.io"}`))
		require.Error(t, err)
	})

	t.Run("errors on versions rewrite", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "groupVersion")
		_, err := RewriteAPIGroup(rules, []byte(`{"name":"prefixed.resources.group.io","versions":[{"groupVersion":"prefixed.resources.group.io/v1"}]}`))
		require.Error(t, err)
	})

	t.Run("errors on preferredVersion rewrite", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "preferredVersion.groupVersion")
		_, err := RewriteAPIGroup(rules, []byte(`{"name":"prefixed.resources.group.io","preferredVersion":{"groupVersion":"prefixed.resources.group.io/v1"}}`))
		require.Error(t, err)
	})

	t.Run("restores group name and versions", func(t *testing.T) {
		obj := []byte(`{"name":"prefixed.resources.group.io","versions":[{"groupVersion":"prefixed.resources.group.io/v1","version":"v1"}],"preferredVersion":{"groupVersion":"prefixed.resources.group.io/v1","version":"v1"}}`)
		res, err := RewriteAPIGroup(rules, obj)
		require.NoError(t, err)
		require.Equal(t, "original.group.io", gjson.GetBytes(res, "name").String())
		require.Equal(t, "original.group.io/v1", gjson.GetBytes(res, "versions.0.groupVersion").String())
		require.Equal(t, "original.group.io/v1", gjson.GetBytes(res, "preferredVersion.groupVersion").String())
	})
}

func TestRewriteAPIResourceList_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("passthrough for non-renamed groupVersion", func(t *testing.T) {
		obj := []byte(`{"groupVersion":"original.group.io/v1"}`)
		res, err := RewriteAPIResourceList(rules, obj)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	base := []byte(`{
  "groupVersion":"prefixed.resources.group.io/v1",
  "resources":[
    {
      "name":"prefixedsomeresources",
      "singularName":"prefixedsomeresource",
      "kind":"PrefixedSomeResource",
      "shortNames":["psr","psrs"],
      "categories":["prefixed"]
    },
    {
      "name":"prefixedunknownresources",
      "singularName":"prefixedunknownresource",
      "kind":"PrefixedUnknownResource",
      "shortNames":["pur"]
    }
  ]
}`)

	t.Run("restores known resources and keeps unknown", func(t *testing.T) {
		res, err := RewriteAPIResourceList(rules, base)
		require.NoError(t, err)
		require.Equal(t, "original.group.io/v1", gjson.GetBytes(res, "groupVersion").String())
		require.Equal(t, "someresources", gjson.GetBytes(res, "resources.0.name").String())
		require.Equal(t, "someresource", gjson.GetBytes(res, "resources.0.singularName").String())
		require.Equal(t, "SomeResource", gjson.GetBytes(res, "resources.0.kind").String())
		require.Equal(t, `["sr","srs"]`, gjson.GetBytes(res, "resources.0.shortNames").Raw)
		require.Equal(t, `["all"]`, gjson.GetBytes(res, "resources.0.categories").Raw)
		require.Equal(t, "prefixedunknownresources", gjson.GetBytes(res, "resources.1.name").String())
	})

	t.Run("errors on groupVersion set", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "groupVersion")
		_, err := RewriteAPIResourceList(rules, base)
		require.Error(t, err)
	})

	tests := []string{"name", "kind", "singularName", "shortNames", "categories"}
	for _, failPath := range tests {
		t.Run("errors on resource rewrite "+failPath, func(t *testing.T) {
			failSjsonSetBytesOnPath(t, failPath)
			_, err := RewriteAPIResourceList(rules, base)
			require.Error(t, err)
		})
	}
}

func TestRewriteAPIGroupDiscoveryList_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("passthrough for empty items", func(t *testing.T) {
		obj := []byte(`{"items":[]}`)
		res, err := RewriteAPIGroupDiscoveryList(rules, obj)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("rewrites renamed items and handles duplicates", func(t *testing.T) {
		obj := []byte(`{
  "items":[
    {"metadata":{"name":"original.group.io","creationTimestamp":null},"versions":[{"version":"v1","resources":[]}]},
    {"metadata":{"name":"some.other.group","creationTimestamp":null},"versions":[]},
    {"metadata":{"name":"prefixed.resources.group.io","creationTimestamp":null},"versions":[]},
    {"metadata":{"name":"prefixed.resources.group.io","creationTimestamp":null},"versions":[
      {"version":"","resources":[{"resource":"prefixedsomeresources"}]},
      {"version":"v1","resources":[]},
      {"version":"v1","freshness":"Current","resources":[
        {"resource":"prefixedsomeresources","singularResource":"prefixedsomeresource","shortNames":["psr","psrs"],"categories":["prefixed"],"responseKind":{"group":"prefixed.resources.group.io","version":"v1","kind":"PrefixedSomeResource"},"subresources":[{"subresource":"status","responseKind":{"group":"prefixed.resources.group.io","version":"v1","kind":"PrefixedSomeResource"}}]},
        {"resource":"prefixedotherresources","singularResource":"prefixedotherresource","shortNames":["por"],"categories":["prefixed"],"responseKind":{"group":"other.prefixed.resources.group.io","version":"v2alpha3","kind":"PrefixedOtherResource"}}
      ]}
    ]}
  ]
}`)
		res, err := RewriteAPIGroupDiscoveryList(rules, obj)
		require.NoError(t, err)

		// Collect returned group names without relying on ordering.
		counts := map[string]int{}
		for _, item := range gjson.GetBytes(res, "items").Array() {
			counts[gjson.Get(item.Raw, "metadata.name").String()]++
		}

		require.Equal(t, 1, counts["original.group.io"], "original group should be present only once (deduplicated)")
		require.Equal(t, 1, counts["some.other.group"], "unrelated group should stay")
		require.Equal(t, 1, counts["prefixed.resources.group.io"], "renamed group with empty versions should stay as-is")
		require.Equal(t, 1, counts["other.group.io"], "restored group from aggregated discovery should be present")
	})

	t.Run("errors on final items set", func(t *testing.T) {
		failSjsonSetRawBytesOnPath(t, "items")
		_, err := RewriteAPIGroupDiscoveryList(rules, []byte(`{"items":[{"metadata":{"name":"some.other.group"},"versions":[]}]}`))
		require.Error(t, err)
	})

	t.Run("returns error from RestoreAggregatedGroupDiscovery", func(t *testing.T) {
		failSjsonSetRawBytesOnPath(t, "resources")
		obj := []byte(`{"items":[{"metadata":{"name":"prefixed.resources.group.io"},"versions":[{"version":"v1","resources":[{"resource":"prefixedsomeresources","responseKind":{"group":"prefixed.resources.group.io","version":"v1","kind":"PrefixedSomeResource"}}]}]}]}`)
		_, err := RewriteAPIGroupDiscoveryList(rules, obj)
		require.Error(t, err)
	})
}

func TestRestoreAggregatedGroupDiscovery_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("nil for empty versions", func(t *testing.T) {
		items, err := RestoreAggregatedGroupDiscovery(rules, []byte(`{"versions":[]}`))
		require.NoError(t, err)
		require.Nil(t, items)
	})

	t.Run("returns nil without error when RestoreAggregatedDiscoveryResource errors", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "resource")
		items, err := RestoreAggregatedGroupDiscovery(rules, []byte(`{"versions":[{"version":"v1","resources":[{"resource":"prefixedsomeresources","responseKind":{"group":"prefixed.resources.group.io","version":"v1","kind":"PrefixedSomeResource"}}]}]}`))
		require.NoError(t, err)
		require.Nil(t, items)
	})

	t.Run("errors on setting resources field", func(t *testing.T) {
		failSjsonSetRawBytesOnPath(t, "resources")
		_, err := RestoreAggregatedGroupDiscovery(rules, []byte(`{"versions":[{"version":"v1","resources":[{"resource":"prefixedsomeresources","responseKind":{"group":"prefixed.resources.group.io","version":"v1","kind":"PrefixedSomeResource"}}]}]}`))
		require.Error(t, err)
	})

	t.Run("errors on setting versions field", func(t *testing.T) {
		failSjsonSetRawBytesOnPath(t, "versions")
		_, err := RestoreAggregatedGroupDiscovery(rules, []byte(`{"versions":[{"version":"v1","resources":[{"resource":"prefixedsomeresources","responseKind":{"group":"prefixed.resources.group.io","version":"v1","kind":"PrefixedSomeResource"}}]}]}`))
		require.Error(t, err)
	})
}

func TestRestoreAggregatedDiscoveryResource_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("ignores resource without rules", func(t *testing.T) {
		group, res, err := RestoreAggregatedDiscoveryResource(rules, []byte(`{"resource":"prefixedunknownresources"}`))
		require.NoError(t, err)
		require.Empty(t, group)
		require.Nil(t, res)
	})

	t.Run("restores all supported fields", func(t *testing.T) {
		obj := []byte(`{
  "resource":"prefixedsomeresources",
  "singularResource":"prefixedsomeresource",
  "shortNames":["psr","psrs"],
  "categories":["prefixed"],
  "responseKind":{"group":"prefixed.resources.group.io","version":"v1","kind":"PrefixedSomeResource"},
  "subresources":[{"subresource":"status","responseKind":{"group":"prefixed.resources.group.io","version":"v1","kind":"PrefixedSomeResource"}}]
}`)
		group, res, err := RestoreAggregatedDiscoveryResource(rules, obj)
		require.NoError(t, err)
		require.Equal(t, "original.group.io", group)
		require.Equal(t, "someresources", gjson.GetBytes(res, "resource").String())
		require.Equal(t, "someresource", gjson.GetBytes(res, "singularResource").String())
		require.Equal(t, `["sr","srs"]`, gjson.GetBytes(res, "shortNames").Raw)
		require.Equal(t, `["all"]`, gjson.GetBytes(res, "categories").Raw)
		require.Equal(t, "original.group.io", gjson.GetBytes(res, "responseKind.group").String())
		require.Equal(t, "SomeResource", gjson.GetBytes(res, "responseKind.kind").String())
		require.Equal(t, "original.group.io", gjson.GetBytes(res, "subresources.0.responseKind.group").String())
		require.Equal(t, "SomeResource", gjson.GetBytes(res, "subresources.0.responseKind.kind").String())
	})

	t.Run("errors on setting resource", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "resource")
		_, _, err := RestoreAggregatedDiscoveryResource(rules, []byte(`{"resource":"prefixedsomeresources"}`))
		require.Error(t, err)
	})

	t.Run("errors on setting responseKind kind", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "responseKind.kind")
		_, _, err := RestoreAggregatedDiscoveryResource(rules, []byte(`{"resource":"prefixedsomeresources","responseKind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"}}`))
		require.Error(t, err)
	})

	t.Run("errors on setting singularResource", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "singularResource")
		_, _, err := RestoreAggregatedDiscoveryResource(rules, []byte(`{"resource":"prefixedsomeresources","singularResource":"prefixedsomeresource"}`))
		require.Error(t, err)
	})

	t.Run("errors on setting shortNames", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "shortNames")
		_, _, err := RestoreAggregatedDiscoveryResource(rules, []byte(`{"resource":"prefixedsomeresources","shortNames":["psr"]}`))
		require.Error(t, err)
	})

	t.Run("errors on setting categories", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "categories")
		_, _, err := RestoreAggregatedDiscoveryResource(rules, []byte(`{"resource":"prefixedsomeresources","categories":["prefixed"]}`))
		require.Error(t, err)
	})

	t.Run("errors on subresources responseKind rewrite", func(t *testing.T) {
		// Skip top-level responseKind rewrite to hit error branch inside subresources callback.
		failSjsonSetBytesOnPath(t, "responseKind.group")
		_, res, err := RestoreAggregatedDiscoveryResource(rules, []byte(`{"resource":"prefixedsomeresources","responseKind":"nope","subresources":[{"responseKind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"}}]}`))
		require.NoError(t, err)
		require.Nil(t, res)
	})

	t.Run("errors on subresources responseKind kind rewrite", func(t *testing.T) {
		// Skip top-level responseKind rewrite to hit error branch inside subresources callback.
		failSjsonSetBytesOnPath(t, "responseKind.kind")
		_, res, err := RestoreAggregatedDiscoveryResource(rules, []byte(`{"resource":"prefixedsomeresources","responseKind":"nope","subresources":[{"responseKind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"}}]}`))
		require.NoError(t, err)
		require.Nil(t, res)
	})

	t.Run("errors on responseKind rewrite", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "responseKind.group")
		_, _, err := RestoreAggregatedDiscoveryResource(rules, []byte(`{"resource":"prefixedsomeresources","responseKind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"}}`))
		require.Error(t, err)
	})
}
