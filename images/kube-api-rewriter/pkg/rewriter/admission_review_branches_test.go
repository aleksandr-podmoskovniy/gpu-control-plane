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
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRenameAdmissionReviewResponse_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("passthrough for non-JSONPatch", func(t *testing.T) {
		obj := []byte(`{"uid":"x","allowed":true,"patchType":"MergePatch","patch":"AA=="}`)
		res, err := RenameAdmissionReviewResponse(rules, obj)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("passthrough for empty patch", func(t *testing.T) {
		obj := []byte(`{"uid":"x","allowed":true,"patchType":"JSONPatch","patch":""}`)
		res, err := RenameAdmissionReviewResponse(rules, obj)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("errors on invalid base64", func(t *testing.T) {
		obj := []byte(`{"uid":"x","allowed":true,"patchType":"JSONPatch","patch":"!!!"}`)
		_, err := RenameAdmissionReviewResponse(rules, obj)
		require.Error(t, err)
	})

	t.Run("errors on rename metadata patch", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "value")

		patch := []byte(`[{"op":"replace","path":"/metadata/labels","value":{"labelgroup.io":"v"}}]`)
		obj := []byte(`{"uid":"x","allowed":true,"patchType":"JSONPatch","patch":"` + base64.StdEncoding.EncodeToString(patch) + `"}`)
		_, err := RenameAdmissionReviewResponse(rules, obj)
		require.Error(t, err)
	})

	t.Run("errors on setting patch field", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "patch")

		patch := []byte(`[{"op":"replace","path":"/metadata/labels","value":{"labelgroup.io":"v"}}]`)
		obj := []byte(`{"uid":"x","allowed":true,"patchType":"JSONPatch","patch":"` + base64.StdEncoding.EncodeToString(patch) + `"}`)
		_, err := RenameAdmissionReviewResponse(rules, obj)
		require.Error(t, err)
	})

	t.Run("rewrites metadata patch and updates base64", func(t *testing.T) {
		patch := []byte(`[{"op":"replace","path":"/metadata/labels","value":{"labelgroup.io":"v"}}]`)
		obj := []byte(`{"uid":"x","allowed":true,"patchType":"JSONPatch","patch":"` + base64.StdEncoding.EncodeToString(patch) + `"}`)
		res, err := RenameAdmissionReviewResponse(rules, obj)
		require.NoError(t, err)

		decoded, err := base64.StdEncoding.DecodeString(gjson.GetBytes(res, "patch").String())
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(decoded, `0.value.replacedlabelgroup\.io`).String())
	})
}

func TestRestoreAdmissionReviewRequest_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	rules.Finalizers.Prefixes = append(rules.Finalizers.Prefixes, MetadataReplaceRule{
		Original: "group.io",
		Renamed:  "replaced.group.io",
	})
	rules.Init()

	t.Run("ignores request for non-renamed group", func(t *testing.T) {
		req := []byte(`{"resource":{"group":"original.group.io","resource":"someresources"},"requestResource":{"group":"original.group.io","resource":"someresources"}}`)
		res, err := RestoreAdmissionReviewRequest(rules, req)
		require.NoError(t, err)
		require.Nil(t, res)
	})

	t.Run("ignores request for non-renamed requestResource group", func(t *testing.T) {
		req := []byte(`{"resource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},"requestResource":{"group":"original.group.io","resource":"someresources"}}`)
		res, err := RestoreAdmissionReviewRequest(rules, req)
		require.NoError(t, err)
		require.Nil(t, res)
	})

	t.Run("returns early when subresource is set", func(t *testing.T) {
		req := []byte(`{
  "resource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
  "requestResource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
  "subresource":"status",
  "kind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"},
  "requestKind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"}
}`)
		res, err := RestoreAdmissionReviewRequest(rules, req)
		require.NoError(t, err)
		require.Equal(t, "original.group.io", gjson.GetBytes(res, "resource.group").String())
		require.Equal(t, "someresources", gjson.GetBytes(res, "resource.resource").String())
		require.Equal(t, "prefixed.resources.group.io", gjson.GetBytes(res, "kind.group").String())
	})

	t.Run("restores kind and objects for known resource", func(t *testing.T) {
		req := []byte(`{
  "resource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
  "requestResource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
  "kind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"},
  "requestKind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"},
  "object":{
    "apiVersion":"prefixed.resources.group.io/v1",
    "kind":"PrefixedSomeResource",
    "metadata":{
      "labels":{"replacedlabelgroup.io":"labelvalue"},
      "annotations":{"replacedanno.io":"annovalue"},
      "finalizers":["replaced.group.io/protection"],
      "ownerReferences":[{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource","name":"o"}]
    }
  },
  "oldObject":{
    "apiVersion":"prefixed.resources.group.io/v1",
    "kind":"PrefixedSomeResource",
    "metadata":{"labels":{"replacedlabelgroup.io":"labelvalue"}}
  }
}`)
		res, err := RestoreAdmissionReviewRequest(rules, req)
		require.NoError(t, err)
		require.Equal(t, "original.group.io", gjson.GetBytes(res, "resource.group").String())
		require.Equal(t, "someresources", gjson.GetBytes(res, "resource.resource").String())
		require.Equal(t, "SomeResource", gjson.GetBytes(res, "kind.kind").String())
		require.Equal(t, "original.group.io", gjson.GetBytes(res, "kind.group").String())
		require.Equal(t, "SomeResource", gjson.GetBytes(res, "requestKind.kind").String())
		require.Equal(t, "original.group.io/v1", gjson.GetBytes(res, "object.apiVersion").String())
		require.Equal(t, "SomeResource", gjson.GetBytes(res, "object.kind").String())
		require.Equal(t, "labelvalue", gjson.GetBytes(res, `object.metadata.labels.labelgroup\.io`).String())
		require.Equal(t, "annovalue", gjson.GetBytes(res, `object.metadata.annotations.annogroup\.io`).String())
		require.Equal(t, `["group.io/protection"]`, gjson.GetBytes(res, "object.metadata.finalizers").Raw)
		require.Equal(t, "original.group.io/v1", gjson.GetBytes(res, "object.metadata.ownerReferences.0.apiVersion").String())
		require.Equal(t, "SomeResource", gjson.GetBytes(res, "object.metadata.ownerReferences.0.kind").String())
	})

	t.Run("returns errors from sjson set bytes", func(t *testing.T) {
		tests := []string{
			"resource.resource",
			"resource.group",
			"requestResource.resource",
			"requestResource.group",
			"kind.kind",
			"kind.group",
			"requestKind.kind",
			"requestKind.group",
		}

		for _, failPath := range tests {
			t.Run(failPath, func(t *testing.T) {
				failSjsonSetBytesOnPath(t, failPath)

				req := []byte(`{
  "resource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
  "requestResource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
  "kind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"},
  "requestKind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"},
  "object":{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource","metadata":{"labels":{"replacedlabelgroup.io":"v"}}},
  "oldObject":{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource","metadata":{"labels":{"replacedlabelgroup.io":"v"}}}
}`)
				_, err := RestoreAdmissionReviewRequest(rules, req)
				require.Error(t, err)
			})
		}
	})

	t.Run("returns errors from TransformObject", func(t *testing.T) {
		t.Run("object", func(t *testing.T) {
			failSjsonSetRawBytesOnPath(t, "object")
			req := []byte(`{
  "resource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
  "requestResource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
  "kind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"},
  "requestKind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"},
  "object":{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource","metadata":{"labels":{"replacedlabelgroup.io":"v"}}},
  "oldObject":{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource","metadata":{"labels":{"replacedlabelgroup.io":"v"}}}
}`)
			_, err := RestoreAdmissionReviewRequest(rules, req)
			require.Error(t, err)
		})

		t.Run("oldObject", func(t *testing.T) {
			failSjsonSetRawBytesOnPath(t, "oldObject")
			req := []byte(`{
  "resource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
  "requestResource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
  "kind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"},
  "requestKind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"},
  "object":{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource","metadata":{"labels":{"replacedlabelgroup.io":"v"}}},
  "oldObject":{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource","metadata":{"labels":{"replacedlabelgroup.io":"v"}}}
}`)
			_, err := RestoreAdmissionReviewRequest(rules, req)
			require.Error(t, err)
		})
	})
}

func TestRewriteAdmissionReview_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("rewrites response", func(t *testing.T) {
		patch := []byte(`[{"op":"replace","path":"/metadata/labels","value":{"labelgroup.io":"v"}}]`)
		obj := []byte(`{"kind":"AdmissionReview","response":{"patchType":"JSONPatch","patch":"` + base64.StdEncoding.EncodeToString(patch) + `"}}`)
		res, err := RewriteAdmissionReview(rules, obj)
		require.NoError(t, err)

		decoded, err := base64.StdEncoding.DecodeString(gjson.GetBytes(res, "response.patch").String())
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(decoded, `0.value.replacedlabelgroup\.io`).String())
	})

	t.Run("restores request", func(t *testing.T) {
		obj := []byte(`{
  "kind":"AdmissionReview",
  "request":{
    "resource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
    "requestResource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
    "kind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"},
    "requestKind":{"group":"prefixed.resources.group.io","kind":"PrefixedSomeResource"},
    "object":{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource","metadata":{"labels":{"replacedlabelgroup.io":"v"}}},
    "oldObject":{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource","metadata":{"labels":{"replacedlabelgroup.io":"v"}}}
  }
}`)
		res, err := RewriteAdmissionReview(rules, obj)
		require.NoError(t, err)
		require.Equal(t, "original.group.io", gjson.GetBytes(res, "request.resource.group").String())
	})

	t.Run("passes through when request is unknown", func(t *testing.T) {
		obj := []byte(`{"kind":"AdmissionReview","request":{"resource":{"group":"original.group.io","resource":"someresources"}}}`)
		res, err := RewriteAdmissionReview(rules, obj)
		require.NoError(t, err)
		require.Equal(t, "original.group.io", gjson.GetBytes(res, "request.resource.group").String())
	})

	t.Run("returns error from RestoreAdmissionReviewRequest", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "resource.resource")
		obj := []byte(`{"kind":"AdmissionReview","request":{"resource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},"requestResource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"}}}`)
		_, err := RewriteAdmissionReview(rules, obj)
		require.Error(t, err)
	})

	t.Run("returns error from setting request field", func(t *testing.T) {
		failSjsonSetRawBytesOnPath(t, "request")
		obj := []byte(`{
  "kind":"AdmissionReview",
  "request":{
    "resource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"},
    "requestResource":{"group":"prefixed.resources.group.io","resource":"prefixedsomeresources"}
  }
}`)
		_, err := RewriteAdmissionReview(rules, obj)
		require.Error(t, err)
	})
}

func TestRestoreAdmissionReviewObject_ErrorBranches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("errors on restoring resource", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "apiVersion")
		obj := []byte(`{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource","metadata":{"labels":{"replacedlabelgroup.io":"v"}}}`)
		_, err := RestoreAdmissionReviewObject(rules, obj)
		require.Error(t, err)
	})

	t.Run("errors on restoring metadata", func(t *testing.T) {
		failSjsonSetRawBytesOnPath(t, "metadata")
		obj := []byte(`{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource","metadata":{"labels":{"replacedlabelgroup.io":"v"}}}`)
		_, err := RestoreAdmissionReviewObject(rules, obj)
		require.Error(t, err)
	})
}
