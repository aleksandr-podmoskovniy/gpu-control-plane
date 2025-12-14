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
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestLoadRules(t *testing.T) {
	t.Run("file not found", func(t *testing.T) {
		_, err := LoadRules(filepath.Join(t.TempDir(), "no-such-file.yaml"))
		require.Error(t, err)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "rules.yaml")
		require.NoError(t, os.WriteFile(path, []byte("{:"), 0o600))

		_, err := LoadRules(path)
		require.Error(t, err)
	})

	t.Run("valid yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "rules.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`
kindPrefix: Prefixed
resourceTypePrefix: prefixed
shortNamePrefix: p
categories: ["prefixed"]
rules:
  original.group.io:
    groupRule:
      group: original.group.io
      versions: ["v1"]
      preferredVersion: v1
      renamed: prefixed.resources.group.io
    resourceRules:
      someresources:
        kind: SomeResource
        listKind: SomeResourceList
        plural: someresources
        singular: someresource
        shortNames: ["sr"]
        categories: ["all"]
        versions: ["v1"]
        preferredVersion: v1
webhooks: {}
labels:
  prefixes:
    - original: labelgroup.io
      renamed: replacedlabelgroup.io
`), 0o600))

		rules, err := LoadRules(path)
		require.NoError(t, err)
		require.NotNil(t, rules)
		rules.Init()
		require.NotNil(t, rules.LabelsRewriter())
	})
}

func TestJSONPatchPathEncodeDecode(t *testing.T) {
	original := "a/b~c"
	encoded := encodeJSONPatchPath(original)
	require.Equal(t, "a~1b~0c", encoded)
	require.Equal(t, original, decodeJSONPatchPath(encoded))
}

func TestRenameMetadataPatch_MergeAndJSON(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules
	rules.Annotations.Prefixes = append(rules.Annotations.Prefixes, MetadataReplaceRule{
		Original: "annogroup.io",
		Renamed:  "replacedanno.io",
	})
	rules.Finalizers.Prefixes = append(rules.Finalizers.Prefixes, MetadataReplaceRule{
		Original: "group.io",
		Renamed:  "replaced.group.io",
	})
	rules.Init()

	t.Run("merge patch", func(t *testing.T) {
		mergePatch := []byte(`{"metadata":{"labels":{"labelgroup.io":"v","labelgroup.io/some-attr":"x"}}}`)
		res, err := RenameMetadataPatch(rules, mergePatch)
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(res, `metadata.labels.replacedlabelgroup\.io`).String())
		require.Equal(t, "x", gjson.GetBytes(res, `metadata.labels.replacedlabelgroup\.io/some-attr`).String())
	})

	t.Run("json patch", func(t *testing.T) {
		patch := []byte(`[
{"op":"replace","path":"/metadata/labels","value":{"labelgroup.io":"labelValue"}},
{"op":"replace","path":"/metadata/annotations","value":{"annogroup.io":"annoValue","annogroup.io/some-attr":"x"}},
{"op":"replace","path":"/metadata/finalizers","value":["group.io/protection"]},
{"op":"replace","path":"/metadata","value":{"labels":{"labelgroup.io":"v"},"annotations":{"annogroup.io":"a"},"finalizers":["group.io/protection"]}},
{"op":"remove","path":"/metadata/labels/labelgroup.io~1some-attr"},
{"op":"remove","path":"/metadata/annotations/annogroup.io~1anno"},
{"op":"remove","path":"/metadata/finalizers/group.io~1protection"},
{"op":"remove","path":"/spec"}
]`)

		res, err := RenameMetadataPatch(rules, patch)
		require.NoError(t, err)
		require.Equal(t, "labelValue", gjson.GetBytes(res, `0.value.replacedlabelgroup\.io`).String())
		require.Equal(t, "annoValue", gjson.GetBytes(res, `1.value.replacedanno\.io`).String())
		require.Equal(t, `["replaced.group.io/protection"]`, gjson.GetBytes(res, `2.value`).Raw)
		require.Equal(t, "v", gjson.GetBytes(res, `3.value.labels.replacedlabelgroup\.io`).String())
		require.Equal(t, "a", gjson.GetBytes(res, `3.value.annotations.replacedanno\.io`).String())
		require.Equal(t, `["replaced.group.io/protection"]`, gjson.GetBytes(res, `3.value.finalizers`).Raw)
		require.Equal(t, "/metadata/labels/replacedlabelgroup.io~1some-attr", gjson.GetBytes(res, "4.path").String())
		require.Equal(t, "/metadata/annotations/replacedanno.io~1anno", gjson.GetBytes(res, "5.path").String())
		require.Equal(t, "/metadata/finalizers/replaced.group.io~1protection", gjson.GetBytes(res, "6.path").String())
		require.Equal(t, "/spec", gjson.GetBytes(res, "7.path").String())
	})
}

func TestPrefixedNameRewriter_RewriteSliceAndMapActions(t *testing.T) {
	rewrite := NewPrefixedNameRewriter(sampleRules())

	require.Nil(t, rewrite.RewriteSlice(nil, Rename))
	require.Nil(t, rewrite.RewriteMap(nil, Rename))

	names := []string{"gpu.deckhouse.io/custom"}
	renamedNames := rewrite.RewriteSlice(names, Rename)
	require.Equal(t, []string{"internal.gpu.deckhouse.io/custom"}, renamedNames)
	require.Equal(t, names, rewrite.RewriteSlice(renamedNames, Restore))

	m := map[string]string{"gpu.deckhouse.io/managed-by": "controller"}
	renamedMap := rewrite.RewriteMap(m, Rename)
	require.Equal(t, map[string]string{"internal.gpu.deckhouse.io/managed-by": "controller"}, renamedMap)
	require.Equal(t, m, rewrite.RewriteMap(renamedMap, Restore))

	// Default action: return as-is.
	require.Equal(t, names, rewrite.RewriteSlice(names, Action("noop")))
	require.Equal(t, m, rewrite.RewriteMap(m, Action("noop")))

	// RewriteNameValues shortcut for empty values.
	name2, values2 := rewrite.RewriteNameValues("gpu.deckhouse.io/custom", nil, Rename)
	require.Equal(t, "internal.gpu.deckhouse.io/custom", name2)
	require.Nil(t, values2)

	name3, values3 := rewrite.RewriteNameValues("gpu.deckhouse.io/custom", []string{}, Rename)
	require.Equal(t, "internal.gpu.deckhouse.io/custom", name3)
	require.Len(t, values3, 0)

	name4, values4 := rewrite.RewriteNameValues("gpu.deckhouse.io/feature", []string{"enabled"}, Action("noop"))
	require.Equal(t, "gpu.deckhouse.io/feature", name4)
	require.Equal(t, []string{"enabled"}, values4)

	n, v := rewrite.RewriteNameValue("gpu.deckhouse.io/custom", "value", Action("noop"))
	require.Equal(t, "gpu.deckhouse.io/custom", n)
	require.Equal(t, "value", v)
}

func TestRewriteRules_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	groups := rules.GetAPIGroupList()
	require.NotEmpty(t, groups)

	{
		gr, rr := rules.KindRules("no-such.group/v1", "SomeResource")
		require.Nil(t, gr)
		require.Nil(t, rr)
	}
	{
		gr, rr := rules.KindRules("original.group.io/v1", "SomeResource")
		require.NotNil(t, gr)
		require.NotNil(t, rr)
	}
	{
		gr, rr := rules.KindRules("original.group.io/v1", "SomeResourceList")
		require.NotNil(t, gr)
		require.NotNil(t, rr)
	}
	{
		gr, rr := rules.KindRules("original.group.io/v1", "NoSuchKind")
		require.Nil(t, gr)
		require.Nil(t, rr)
	}
	{
		gr, rr := rules.GroupResourceRules("someresources/status")
		require.NotNil(t, gr)
		require.NotNil(t, rr)
	}
	{
		gr, rr := rules.GroupResourceRules("no-such-resource")
		require.Nil(t, gr)
		require.Nil(t, rr)
	}
	{
		gr, rr := rules.GroupResourceRulesByKind("SomeResource")
		require.NotNil(t, gr)
		require.NotNil(t, rr)
	}
	{
		gr, rr := rules.GroupResourceRulesByKind("NoSuchKind")
		require.Nil(t, gr)
		require.Nil(t, rr)
	}
	require.Equal(t, []string{}, rules.RenameCategories(nil))
	require.Equal(t, rules.Categories, rules.RenameCategories([]string{"any"}))
	require.Equal(t, []string{}, rules.RestoreCategories(nil))

	require.Equal(t, "psr", rules.RenameShortName("sr"))

	require.True(t, mapContainsMap(map[string]string{"a": "b"}, map[string]string{}))
	require.False(t, mapContainsMap(map[string]string{"a": "b"}, map[string]string{"a": "c"}))
}

func TestRewriteArray_Branches(t *testing.T) {
	obj := []byte(`{"kind":"X","items":[{"a":1},{"a":2}]}`)

	// Replacement, passthrough (nil), skip, and error.
	var seen []int
	res, err := RewriteArray(obj, "items", func(item []byte) ([]byte, error) {
		a := int(gjson.GetBytes(item, "a").Int())
		seen = append(seen, a)
		switch a {
		case 1:
			return []byte(`{"a":10}`), nil
		case 2:
			return nil, SkipItem
		default:
			return nil, errors.New("unexpected")
		}
	})
	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, seen)
	require.Equal(t, `[{"a":10}]`, gjson.GetBytes(res, "items").Raw)

	_, err = RewriteArray(obj, "items", func([]byte) ([]byte, error) {
		return nil, errors.New("boom")
	})
	require.Error(t, err)
}

func TestTransformPatch_Branches(t *testing.T) {
	empty, err := TransformPatch(nil, func([]byte) ([]byte, error) {
		t.Fatalf("unexpected merge transform call")
		return nil, nil
	}, func([]byte) ([]byte, error) {
		t.Fatalf("unexpected json transform call")
		return nil, nil
	})
	require.NoError(t, err)
	require.Nil(t, empty)

	mergeRes, err := TransformPatch([]byte(`{"a":1}`), func([]byte) ([]byte, error) {
		return []byte(`{"a":2}`), nil
	}, nil)
	require.NoError(t, err)
	require.Equal(t, `{"a":2}`, string(mergeRes))

	jsonRes, err := TransformPatch([]byte(`[{"path":"/x"}]`), nil, func(item []byte) ([]byte, error) {
		return item, nil
	})
	require.NoError(t, err)
	require.Equal(t, `[{"path":"/x"}]`, string(jsonRes))

	otherRes, err := TransformPatch([]byte(`"not a patch"`), func([]byte) ([]byte, error) {
		return []byte(`"merge"`), nil
	}, func([]byte) ([]byte, error) {
		return []byte(`"json"`), nil
	})
	require.NoError(t, err)
	require.Equal(t, `"not a patch"`, string(otherRes))
}

func TestTransformObject_Branches(t *testing.T) {
	obj := []byte(`{"a":{"b":1},"c":1}`)

	// Path is not an object: return as-is.
	res, err := TransformObject(obj, "c", func([]byte) ([]byte, error) {
		t.Fatalf("unexpected transform call")
		return nil, nil
	})
	require.NoError(t, err)
	require.Equal(t, obj, res)

	// Error from transformFn.
	_, err = TransformObject(obj, "a", func([]byte) ([]byte, error) {
		return nil, errors.New("boom")
	})
	require.Error(t, err)
}

func TestTargetRequest_Branches(t *testing.T) {
	rwr := createTestRewriter()

	require.Nil(t, NewTargetRequest(rwr, nil))
	require.Nil(t, NewTargetRequest(rwr, &http.Request{}))

	{
		tr := &TargetRequest{}
		require.Equal(t, "", tr.Path())
		require.Equal(t, "", tr.OrigGroup())
		require.Equal(t, "", tr.OrigResourceType())
		require.Equal(t, "", tr.RawQuery())
		require.False(t, tr.IsWebhook())
		require.False(t, tr.IsCore())
		require.False(t, tr.IsWatch())
		require.False(t, tr.ShouldRewriteResponse())
		require.Equal(t, "UNKNOWN", tr.ResourceForLog())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/validate-prefixed-resources-group-io-v1-prefixedsomeresource"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.True(t, tr.IsWebhook())
		require.Equal(t, "/validate-original-group-io-v1-someresource", tr.Path())
		require.Equal(t, "original.group.io", tr.OrigGroup())
		require.Equal(t, "someresources", tr.OrigResourceType())
		require.True(t, tr.ShouldRewriteRequest())
		require.True(t, tr.ShouldRewriteResponse())
		require.Equal(t, "someresources", tr.ResourceForLog())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.False(t, tr.ShouldRewriteRequest())
		require.False(t, tr.ShouldRewriteResponse())
		require.Equal(t, "ROOT", tr.ResourceForLog())
		require.Equal(t, "/", tr.RequestURI())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/foo"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.False(t, tr.ShouldRewriteRequest())
		require.False(t, tr.ShouldRewriteResponse())
		require.Equal(t, "UKNOWN", tr.ResourceForLog())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/api"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.True(t, tr.IsCore())
		require.False(t, tr.ShouldRewriteRequest())
		require.True(t, tr.ShouldRewriteResponse())
		require.Equal(t, "APIVersions/core", tr.ResourceForLog())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/api/v1"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.True(t, tr.IsCore())
		require.False(t, tr.IsWatch())
		require.Equal(t, "APIResourceList/core", tr.ResourceForLog())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/api/v1/pods", RawQuery: "watch=true"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.True(t, tr.IsCore())
		require.True(t, tr.IsWatch())
		require.Equal(t, "pods", tr.ResourceForLog())
		require.Equal(t, "/api/v1/pods?watch=true", tr.RequestURI())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/api/v1/pods/p1/status"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.Equal(t, "pods/status", tr.ResourceForLog())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/apis"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.False(t, tr.ShouldRewriteRequest())
		require.True(t, tr.ShouldRewriteResponse())
		require.Equal(t, "APIGroupList", tr.ResourceForLog())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/apis/original.group.io/v1"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.Equal(t, "APIResourceList/original.group.io", tr.ResourceForLog())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/apis/original.group.io/v1/someresources"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.Equal(t, "someresources", tr.ResourceForLog())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/apis/original.group.io/v1/someresources/s1/status"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.Equal(t, "someresources/status", tr.ResourceForLog())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/apis/original.group.io", RawQuery: "x=y"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.Equal(t, "/apis/prefixed.resources.group.io", tr.Path())
		require.Equal(t, "/apis/prefixed.resources.group.io?x=y", tr.RequestURI())
		require.Equal(t, "APIGroup/original.group.io", tr.ResourceForLog())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/apis/apiextensions.k8s.io/v1/customresourcedefinitions"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.True(t, tr.IsCRD())
		require.Equal(t, "", tr.OrigGroup())
		require.Equal(t, "", tr.OrigResourceType())
		require.True(t, tr.ShouldRewriteRequest())
		require.True(t, tr.ShouldRewriteResponse())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/someresources.original.group.io"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.True(t, tr.IsCRD())
		require.Equal(t, "original.group.io", tr.OrigGroup())
		require.Equal(t, "someresources", tr.OrigResourceType())
		require.True(t, tr.ShouldRewriteResponse())
	}

	{
		req := &http.Request{URL: &url.URL{Path: "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/resource.unknown.group.io"}}
		tr := NewTargetRequest(rwr, req)
		require.NotNil(t, tr)
		require.True(t, tr.IsCRD())
		require.Equal(t, "unknown.group.io", tr.OrigGroup())
		require.Equal(t, "resource", tr.OrigResourceType())
		require.False(t, tr.ShouldRewriteRequest())
		require.False(t, tr.ShouldRewriteResponse())
	}

	{
		tr := &TargetRequest{originEndpoint: &APIEndpoint{ResourceType: "customresourcedefinitions"}}
		require.True(t, tr.ShouldRewriteRequest())
	}

	{
		tr := &TargetRequest{}
		require.True(t, tr.ShouldRewriteRequest())
	}

	{
		tr := &TargetRequest{originEndpoint: &APIEndpoint{ResourceType: "pods"}}
		require.True(t, tr.ShouldRewriteRequest())
		require.True(t, tr.ShouldRewriteResponse())
	}

	{
		tr := &TargetRequest{originEndpoint: &APIEndpoint{Group: "some.group.io", ResourceType: "no-such"}}
		require.False(t, tr.ShouldRewriteRequest())
		require.False(t, tr.ShouldRewriteResponse())
	}
}

func TestRewriteJSONPayload_CoversMoreKinds(t *testing.T) {
	rwr := createTestRewriter()

	tests := []struct {
		name   string
		kind   string
		action Action
		body   []byte
	}{
		{
			name:   "mutating webhook",
			kind:   MutatingWebhookConfigurationKind,
			action: Rename,
			body:   []byte(`{"kind":"MutatingWebhookConfiguration","webhooks":[{"rules":[{"apiGroups":["original.group.io"],"resources":["someresources"],"verbs":["get"]}]}]}`),
		},
		{
			name:   "mutating webhook restore",
			kind:   MutatingWebhookConfigurationKind,
			action: Restore,
			body:   []byte(`{"kind":"MutatingWebhookConfiguration","webhooks":[{"rules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}]}`),
		},
		{
			name:   "mutating webhook list",
			kind:   MutatingWebhookConfigurationListKind,
			action: Rename,
			body:   []byte(`{"kind":"MutatingWebhookConfigurationList","items":[{"kind":"MutatingWebhookConfiguration","webhooks":[{"rules":[{"apiGroups":["original.group.io"],"resources":["someresources"],"verbs":["get"]}]}]}]}`),
		},
		{
			name:   "mutating webhook list restore",
			kind:   MutatingWebhookConfigurationListKind,
			action: Restore,
			body:   []byte(`{"kind":"MutatingWebhookConfigurationList","items":[{"kind":"MutatingWebhookConfiguration","webhooks":[{"rules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}]}]}`),
		},
		{
			name:   "validating webhook",
			kind:   ValidatingWebhookConfigurationKind,
			action: Restore,
			body:   []byte(`{"kind":"ValidatingWebhookConfiguration","webhooks":[{"rules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}]}`),
		},
		{
			name:   "validating webhook list",
			kind:   ValidatingWebhookConfigurationListKind,
			action: Restore,
			body:   []byte(`{"kind":"ValidatingWebhookConfigurationList","items":[{"kind":"ValidatingWebhookConfiguration","webhooks":[{"rules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}]}]}`),
		},
		{
			name:   "cluster role",
			kind:   ClusterRoleKind,
			action: Rename,
			body:   []byte(`{"kind":"ClusterRole","rules":[{"apiGroups":["original.group.io"],"resources":["someresources"],"verbs":["get"]}]}`),
		},
		{
			name:   "cluster role restore",
			kind:   ClusterRoleKind,
			action: Restore,
			body:   []byte(`{"kind":"ClusterRole","rules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}`),
		},
		{
			name:   "cluster role list",
			kind:   ClusterRoleListKind,
			action: Rename,
			body:   []byte(`{"kind":"ClusterRoleList","items":[{"kind":"ClusterRole","rules":[{"apiGroups":["original.group.io"],"resources":["someresources"],"verbs":["get"]}]}]}`),
		},
		{
			name:   "cluster role list restore",
			kind:   ClusterRoleListKind,
			action: Restore,
			body:   []byte(`{"kind":"ClusterRoleList","items":[{"kind":"ClusterRole","rules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}]}`),
		},
		{
			name:   "role",
			kind:   RoleKind,
			action: Rename,
			body:   []byte(`{"kind":"Role","rules":[{"apiGroups":["original.group.io"],"resources":["someresources"],"verbs":["get"]}]}`),
		},
		{
			name:   "role restore",
			kind:   RoleKind,
			action: Restore,
			body:   []byte(`{"kind":"Role","rules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}`),
		},
		{
			name:   "role list",
			kind:   RoleListKind,
			action: Rename,
			body:   []byte(`{"kind":"RoleList","items":[{"kind":"Role","rules":[{"apiGroups":["original.group.io"],"resources":["someresources"],"verbs":["get"]}]}]}`),
		},
		{
			name:   "role list restore",
			kind:   RoleListKind,
			action: Restore,
			body:   []byte(`{"kind":"RoleList","items":[{"kind":"Role","rules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}]}`),
		},
		{
			name:   "daemonset",
			kind:   DaemonSetKind,
			action: Rename,
			body:   []byte(`{"kind":"DaemonSet","spec":{"selector":{"matchLabels":{"labelgroup.io":"v"}},"template":{"metadata":{"labels":{"labelgroup.io":"v"}},"spec":{"nodeSelector":{"labelgroup.io":"v"}}}}}`),
		},
		{
			name:   "statefulset",
			kind:   StatefulSetKind,
			action: Rename,
			body:   []byte(`{"kind":"StatefulSet","spec":{"selector":{"matchLabels":{"labelgroup.io":"v"}},"template":{"metadata":{"labels":{"labelgroup.io":"v"}},"spec":{"nodeSelector":{"labelgroup.io":"v"}}}}}`),
		},
		{
			name:   "pod",
			kind:   PodKind,
			action: Rename,
			body:   []byte(`{"kind":"Pod","spec":{"nodeSelector":{"labelgroup.io":"v"}}}`),
		},
		{
			name:   "service",
			kind:   ServiceKind,
			action: Rename,
			body:   []byte(`{"kind":"Service","spec":{"selector":{"labelgroup.io":"v"}}}`),
		},
		{
			name:   "job",
			kind:   JobKind,
			action: Rename,
			body:   []byte(`{"kind":"Job","spec":{"template":{"metadata":{"labels":{"labelgroup.io":"v"}},"spec":{"nodeSelector":{"labelgroup.io":"v"}}}}}`),
		},
		{
			name:   "pdb",
			kind:   PodDisruptionBudgetKind,
			action: Rename,
			body:   []byte(`{"kind":"PodDisruptionBudget","spec":{"selector":{"matchLabels":{"labelgroup.io":"v"}}}}`),
		},
		{
			name:   "service monitor",
			kind:   ServiceMonitorKind,
			action: Rename,
			body:   []byte(`{"kind":"ServiceMonitor","spec":{"selector":{"matchLabels":{"labelgroup.io":"v"},"matchExpressions":[{"key":"labelgroup.io","operator":"In","values":["v"]}]}}}`),
		},
		{
			name:   "validating admission policy",
			kind:   ValidatingAdmissionPolicyKind,
			action: Rename,
			body:   []byte(`{"kind":"ValidatingAdmissionPolicy","spec":{"matchConstraints":{"resourceRules":[{"apiGroups":["original.group.io"],"resources":["someresources"],"verbs":["get"]}]}}}`),
		},
		{
			name:   "validating admission policy restore",
			kind:   ValidatingAdmissionPolicyKind,
			action: Restore,
			body:   []byte(`{"kind":"ValidatingAdmissionPolicy","spec":{"matchConstraints":{"resourceRules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}}}`),
		},
		{
			name:   "validating admission policy list",
			kind:   ValidatingAdmissionPolicyListKind,
			action: Rename,
			body:   []byte(`{"kind":"ValidatingAdmissionPolicyList","items":[{"kind":"ValidatingAdmissionPolicy","spec":{"matchConstraints":{"resourceRules":[{"apiGroups":["original.group.io"],"resources":["someresources"],"verbs":["get"]}]}}}]}`),
		},
		{
			name:   "validating admission policy list restore",
			kind:   ValidatingAdmissionPolicyListKind,
			action: Restore,
			body:   []byte(`{"kind":"ValidatingAdmissionPolicyList","items":[{"kind":"ValidatingAdmissionPolicy","spec":{"matchConstraints":{"resourceRules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}}}]}`),
		},
		{
			name:   "validating admission policy binding",
			kind:   ValidatingAdmissionPolicyBindingKind,
			action: Rename,
			body:   []byte(`{"kind":"ValidatingAdmissionPolicyBinding","spec":{"matchResources":{"resourceRules":[{"apiGroups":["original.group.io"],"resources":["someresources"],"verbs":["get"]}]}}}`),
		},
		{
			name:   "validating admission policy binding restore",
			kind:   ValidatingAdmissionPolicyBindingKind,
			action: Restore,
			body:   []byte(`{"kind":"ValidatingAdmissionPolicyBinding","spec":{"matchResources":{"resourceRules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}}}`),
		},
		{
			name:   "validating admission policy binding list",
			kind:   ValidatingAdmissionPolicyBindingListKind,
			action: Rename,
			body:   []byte(`{"kind":"ValidatingAdmissionPolicyBindingList","items":[{"kind":"ValidatingAdmissionPolicyBinding","spec":{"matchResources":{"resourceRules":[{"apiGroups":["original.group.io"],"resources":["someresources"],"verbs":["get"]}]}}}]}`),
		},
		{
			name:   "validating admission policy binding list restore",
			kind:   ValidatingAdmissionPolicyBindingListKind,
			action: Restore,
			body:   []byte(`{"kind":"ValidatingAdmissionPolicyBindingList","items":[{"kind":"ValidatingAdmissionPolicyBinding","spec":{"matchResources":{"resourceRules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}}}]}`),
		},
		{
			name:   "pvc",
			kind:   PersistentVolumeClaimKind,
			action: Rename,
			body:   []byte(`{"kind":"PersistentVolumeClaim","spec":{"dataSource":{"apiGroup":"original.group.io","kind":"SomeResource"},"dataSourceRef":{"apiGroup":"original.group.io","kind":"SomeResource"}}}`),
		},
		{
			name:   "event",
			kind:   EventKind,
			action: Rename,
			body:   []byte(`{"kind":"Event","involvedObject":{"apiVersion":"original.group.io/v1","kind":"SomeResource"}}`),
		},
		{
			name:   "custom resource default case",
			kind:   "NoRulesKind",
			action: Rename,
			body:   []byte(`{"kind":"NoRulesKind"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := rwr.RewriteJSONPayload(nil, tt.body, tt.action)
			require.NoError(t, err)
			require.NotNil(t, res)
			require.Equal(t, tt.kind, gjson.GetBytes(res, "kind").String())
		})
	}

	{
		res, err := rwr.RestoreBookmark(nil, []byte(`{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource"}`))
		require.NoError(t, err)
		require.Equal(t, "original.group.io/v1", gjson.GetBytes(res, "apiVersion").String())
		require.Equal(t, "SomeResource", gjson.GetBytes(res, "kind").String())
	}

	{
		// FilterExcludes should return SkipItem for original (unprefixed) kinds.
		_, err := rwr.RewriteJSONPayload(nil, []byte(`{"apiVersion":"original.group.io/v1","kind":"SomeResource"}`), Restore)
		require.ErrorIs(t, err, SkipItem)
	}
}

func TestRewritePatch_Branches(t *testing.T) {
	rwr := createTestRewriter()

	{
		targetReq := &TargetRequest{originEndpoint: &APIEndpoint{
			IsCRD:           true,
			CRDGroup:        "original.group.io",
			CRDResourceType: "someresources",
		}}
		patch := []byte(`[{"op":"replace","path":"/spec","value":{"group":"original.group.io","names":{"plural":"someresources","kind":"SomeResource","shortNames":["sr"]}}}]`)
		res, err := rwr.RewritePatch(targetReq, patch)
		require.NoError(t, err)
		require.Contains(t, string(res), "prefixed.resources.group.io")
	}

	{
		targetReq := &TargetRequest{originEndpoint: &APIEndpoint{
			Group:        "original.group.io",
			ResourceType: "someresources",
		}}
		patch := []byte(`[{"op":"replace","path":"/metadata/labels","value":{"labelgroup.io":"v"}}]`)
		res, err := rwr.RewritePatch(targetReq, patch)
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(res, `0.value.replacedlabelgroup\.io`).String())
	}

	{
		targetReq := &TargetRequest{originEndpoint: &APIEndpoint{ResourceType: "services"}}
		patch := []byte(`[{"op":"replace","path":"/spec","value":{"selector":{"labelgroup.io":"v"}}}]`)
		res, err := rwr.RewritePatch(targetReq, patch)
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(res, `0.value.selector.replacedlabelgroup\.io`).String())
	}

	{
		targetReq := &TargetRequest{originEndpoint: &APIEndpoint{ResourceType: "deployments"}}
		patch := []byte(`[{"op":"replace","path":"/spec","value":{"selector":{"matchLabels":{"labelgroup.io":"v"}},"template":{"metadata":{"labels":{"labelgroup.io":"v"}}}}}]`)
		res, err := rwr.RewritePatch(targetReq, patch)
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(res, `0.value.selector.matchLabels.replacedlabelgroup\.io`).String())
	}

	{
		targetReq := &TargetRequest{originEndpoint: &APIEndpoint{ResourceType: "mutatingwebhookconfigurations"}}
		patch := []byte(`[{"op":"replace","path":"/webhooks","value":[{"rules":[{"apiGroups":["original.group.io"],"resources":["someresources"]}]}]}]`)
		_, err := rwr.RewritePatch(targetReq, patch)
		require.NoError(t, err)
	}

	{
		targetReq := &TargetRequest{originEndpoint: &APIEndpoint{ResourceType: "other"}}
		patch := []byte(`[{"op":"replace","path":"/metadata/labels","value":{"labelgroup.io":"v"}}]`)
		_, err := rwr.RewritePatch(targetReq, patch)
		require.NoError(t, err)
	}
}

func TestCustomResourceList_RenameRestoreAndManagedFields(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	originalList := []byte(`{
  "apiVersion":"original.group.io/v1",
  "kind":"SomeResourceList",
  "items":[
    {
      "apiVersion":"original.group.io/v1",
      "kind":"SomeResource",
      "metadata":{"managedFields":[{"apiVersion":"original.group.io/v1"},{"fieldsType":"FieldsV1"}]}
    }
  ]
}`)

	renamed, err := RewriteCustomResourceOrList(rules, originalList, Rename)
	require.NoError(t, err)
	require.Equal(t, "prefixed.resources.group.io/v1", gjson.GetBytes(renamed, "apiVersion").String())
	require.Equal(t, "PrefixedSomeResourceList", gjson.GetBytes(renamed, "kind").String())
	require.Equal(t, "prefixed.resources.group.io/v1", gjson.GetBytes(renamed, "items.0.apiVersion").String())
	require.Equal(t, "PrefixedSomeResource", gjson.GetBytes(renamed, "items.0.kind").String())
	require.Equal(t, "prefixed.resources.group.io/v1", gjson.GetBytes(renamed, "items.0.metadata.managedFields.0.apiVersion").String())
	require.Equal(t, "", gjson.GetBytes(renamed, "items.0.metadata.managedFields.1.apiVersion").String())

	restored, err := RewriteCustomResourceOrList(rules, renamed, Restore)
	require.NoError(t, err)
	require.Equal(t, "original.group.io/v1", gjson.GetBytes(restored, "apiVersion").String())
	require.Equal(t, "SomeResourceList", gjson.GetBytes(restored, "kind").String())
	require.Equal(t, "original.group.io/v1", gjson.GetBytes(restored, "items.0.apiVersion").String())
	require.Equal(t, "SomeResource", gjson.GetBytes(restored, "items.0.kind").String())
	require.Equal(t, "original.group.io/v1", gjson.GetBytes(restored, "items.0.metadata.managedFields.0.apiVersion").String())
}
