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
	"encoding/base64"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRewriteCRApiEndpoint_Branches(t *testing.T) {
	rw := createTestRewriter()

	t.Run("nil for empty group", func(t *testing.T) {
		require.Nil(t, rw.rewriteCRApiEndpoint(&APIEndpoint{Group: ""}))
	})

	t.Run("nil for unknown group", func(t *testing.T) {
		require.Nil(t, rw.rewriteCRApiEndpoint(&APIEndpoint{Group: "unknown.group.io"}))
	})

	t.Run("nil for unknown resourceType", func(t *testing.T) {
		require.Nil(t, rw.rewriteCRApiEndpoint(&APIEndpoint{Group: "original.group.io", ResourceType: "unknownresources"}))
	})

	t.Run("rewrites group-only request", func(t *testing.T) {
		res := rw.rewriteCRApiEndpoint(&APIEndpoint{Group: "original.group.io"})
		require.NotNil(t, res)
		require.Equal(t, "prefixed.resources.group.io", res.Group)
		require.Empty(t, res.ResourceType)
	})

	t.Run("rewrites group and resourceType", func(t *testing.T) {
		res := rw.rewriteCRApiEndpoint(&APIEndpoint{Group: "original.group.io", ResourceType: "someresources"})
		require.NotNil(t, res)
		require.Equal(t, "prefixed.resources.group.io", res.Group)
		require.Equal(t, "prefixedsomeresources", res.ResourceType)
	})

	t.Run("returns nil when rename produces empty group", func(t *testing.T) {
		rw2 := createTestRewriter()
		grp := rw2.Rules.Rules["original.group.io"]
		grp.GroupRule.Renamed = ""
		rw2.Rules.Rules["original.group.io"] = grp
		rw2.Rules.Init()

		require.Nil(t, rw2.rewriteCRApiEndpoint(&APIEndpoint{Group: "original.group.io"}))
	})
}

func TestRewriteFieldSelector_Branches(t *testing.T) {
	rw := createTestRewriter()

	require.Empty(t, rw.rewriteFieldSelector("watch=true"))
	require.Empty(t, rw.rewriteFieldSelector("fieldSelector=metadata.name%3Dunknownresources.original.group.io"))

	out := rw.rewriteFieldSelector("fieldSelector=metadata.name%3Dsomeresources.original.group.io&watch=true")
	require.Contains(t, out, "prefixedsomeresources.prefixed.resources.group.io")
}

func TestRewriteLabelSelector_Branches(t *testing.T) {
	rw := createTestRewriter()

	t.Run("parse query error passthrough", func(t *testing.T) {
		raw := "labelSelector=%zz"
		require.Equal(t, raw, rw.rewriteLabelSelector(raw))
	})

	t.Run("no labelSelector passthrough", func(t *testing.T) {
		raw := "limit=10"
		require.Equal(t, raw, rw.rewriteLabelSelector(raw))
	})

	t.Run("empty labelSelector passthrough", func(t *testing.T) {
		raw := "labelSelector=&limit=10"
		require.Equal(t, raw, rw.rewriteLabelSelector(raw))
	})

	t.Run("invalid labelSelector passthrough", func(t *testing.T) {
		raw := "labelSelector=,"
		require.Equal(t, raw, rw.rewriteLabelSelector(raw))
	})

	t.Run("LabelSelectorAsSelector error passthrough", func(t *testing.T) {
		rw2 := createTestRewriter()
		rw2.Rules.Labels.Names = append(rw2.Rules.Labels.Names, MetadataReplaceRule{
			Original: "foo",
			Renamed:  "INVALID KEY",
		})
		rw2.Rules.Init()

		raw := "labelSelector=foo=bar"
		require.Equal(t, raw, rw2.rewriteLabelSelector(raw))
	})

	t.Run("rewrites matchLabels and matchExpressions", func(t *testing.T) {
		raw := "labelSelector=labelgroup.io=labelValueToRename,labelgroup.io in (labelValueToRename,other)&limit=10"
		out := rw.rewriteLabelSelector(raw)

		q, err := url.ParseQuery(out)
		require.NoError(t, err)
		lsq := q.Get("labelSelector")
		require.Contains(t, lsq, "replacedlabelgroup.io")
		require.Contains(t, lsq, "renamedLabelValue")
	})

	t.Run("ParseToLabelSelector does not return nil (document behavior)", func(t *testing.T) {
		ls, err := metav1.ParseToLabelSelector(" ")
		require.NoError(t, err)
		require.NotNil(t, ls)
	})
}

func TestFilterExcludes_Branches(t *testing.T) {
	rw := createTestRewriter()

	t.Run("no-op for non-restore action", func(t *testing.T) {
		obj := []byte(`{"kind":"Pod"}`)
		res, err := rw.FilterExcludes(obj, Rename)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("no-op for non-excludable kind", func(t *testing.T) {
		obj := []byte(`{"kind":"APIGroupList"}`)
		res, err := rw.FilterExcludes(obj, Restore)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("returns SkipItem for excluded resource", func(t *testing.T) {
		_, err := rw.FilterExcludes([]byte(`{"kind":"SomeResource"}`), Restore)
		require.ErrorIs(t, err, SkipItem)
	})

	t.Run("no-op for non-list and non-excluded", func(t *testing.T) {
		rw2 := createTestRewriter()
		rw2.Rules.Excludes = nil
		obj := []byte(`{"kind":"Pod","metadata":{"name":"p"}}`)
		res, err := rw2.FilterExcludes(obj, Restore)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("filters excluded items from list", func(t *testing.T) {
		rw2 := createTestRewriter()
		rw2.Rules.Excludes = []ExcludeRule{{MatchNames: []string{"drop-me"}}}

		obj := []byte(`{"kind":"PodList","items":[{"metadata":{"name":"drop-me"}},{"metadata":{"name":"keep-me"}}]}`)
		res, err := rw2.FilterExcludes(obj, Restore)
		require.NoError(t, err)
		require.Contains(t, string(res), "keep-me")
		require.NotContains(t, string(res), "drop-me")
	})

	t.Run("returns error from RewriteResourceOrList2", func(t *testing.T) {
		rw2 := createTestRewriter()
		rw2.Rules.Excludes = nil
		failSjsonSetRawBytesOnPath(t, "items")
		_, err := rw2.FilterExcludes([]byte(`{"kind":"PodList","items":[{"metadata":{"name":"a"}}]}`), Restore)
		require.Error(t, err)
	})
}

func TestRewriteJSONPayload_DiscoveryAndAdmissionReviewKinds(t *testing.T) {
	rw := createTestRewriter()

	t.Run("APIGroupList", func(t *testing.T) {
		obj := []byte(`{"kind":"APIGroupList","groups":[{"name":"prefixed.resources.group.io","versions":[{"groupVersion":"prefixed.resources.group.io/v1","version":"v1"}],"preferredVersion":{"groupVersion":"prefixed.resources.group.io/v1","version":"v1"}}]}`)
		res, err := rw.RewriteJSONPayload(nil, obj, Rename)
		require.NoError(t, err)
		require.Contains(t, string(res), "original.group.io")
	})

	t.Run("APIGroup", func(t *testing.T) {
		obj := []byte(`{"kind":"APIGroup","name":"prefixed.resources.group.io","versions":[{"groupVersion":"prefixed.resources.group.io/v1","version":"v1"}],"preferredVersion":{"groupVersion":"prefixed.resources.group.io/v1","version":"v1"}}`)
		res, err := rw.RewriteJSONPayload(nil, obj, Rename)
		require.NoError(t, err)
		require.Contains(t, string(res), "original.group.io")
	})

	t.Run("APIResourceList", func(t *testing.T) {
		obj := []byte(`{"kind":"APIResourceList","groupVersion":"prefixed.resources.group.io/v1","resources":[{"name":"prefixedsomeresources","kind":"PrefixedSomeResource"}]}`)
		res, err := rw.RewriteJSONPayload(nil, obj, Rename)
		require.NoError(t, err)
		require.Contains(t, string(res), `"name":"someresources"`)
	})

	t.Run("APIGroupDiscoveryList", func(t *testing.T) {
		obj := []byte(`{"kind":"APIGroupDiscoveryList","items":[{"metadata":{"name":"prefixed.resources.group.io","creationTimestamp":null},"versions":[{"version":"v1","resources":[{"resource":"prefixedsomeresources","responseKind":{"group":"prefixed.resources.group.io","version":"v1","kind":"PrefixedSomeResource"}}]}]}]}`)
		res, err := rw.RewriteJSONPayload(nil, obj, Rename)
		require.NoError(t, err)
		require.Contains(t, string(res), "original.group.io")
	})

	t.Run("AdmissionReview", func(t *testing.T) {
		obj := []byte(`{"kind":"AdmissionReview","response":{"patchType":"JSONPatch","patch":"W3sib3AiOiJyZXBsYWNlIiwicGF0aCI6Ii9tZXRhZGF0YS9sYWJlbHMiLCJ2YWx1ZSI6eyJsYWJlbGdyb3VwLmlvIjoidiJ9fV0="}}`)
		res, err := rw.RewriteJSONPayload(nil, obj, Rename)
		require.NoError(t, err)

		decoded, err := base64.StdEncoding.DecodeString(gjson.GetBytes(res, "response.patch").String())
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(decoded, `0.value.replacedlabelgroup\.io`).String())
	})

	t.Run("propagates errors from underlying rewriters", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "name")
		_, err := rw.RewriteJSONPayload(nil, []byte(`{"kind":"APIGroup","name":"prefixed.resources.group.io"}`), Rename)
		require.Error(t, err)
	})
}
