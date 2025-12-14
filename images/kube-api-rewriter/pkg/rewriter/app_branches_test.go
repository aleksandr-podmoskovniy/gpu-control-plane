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

func TestRenameSpecTemplatePatch_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("merge patch", func(t *testing.T) {
		patch := []byte(`{"spec":{"selector":{"matchLabels":{"labelgroup.io":"v"}},"template":{"metadata":{"labels":{"labelgroup.io":"v"}},"spec":{"nodeSelector":{"labelgroup.io":"v"}}}}}`)
		res, err := RenameSpecTemplatePatch(rules, patch)
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(res, `spec.selector.matchLabels.replacedlabelgroup\.io`).String())
		require.Equal(t, "v", gjson.GetBytes(res, `spec.template.metadata.labels.replacedlabelgroup\.io`).String())
		require.Equal(t, "v", gjson.GetBytes(res, `spec.template.spec.nodeSelector.replacedlabelgroup\.io`).String())
	})

	t.Run("json patch rewrites /spec", func(t *testing.T) {
		patch := []byte(`[{"op":"replace","path":"/spec","value":{"selector":{"matchLabels":{"labelgroup.io":"v"}},"template":{"metadata":{"labels":{"labelgroup.io":"v"},"annotations":{"annogroup.io":"a"}},"spec":{"nodeSelector":{"labelgroup.io":"v"},"affinity":{"podAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":[{"labelSelector":{"matchLabels":{"labelgroup.io":"v"}},"topologyKey":"labelgroup.io"}]}}}}}}]`)
		res, err := RenameSpecTemplatePatch(rules, patch)
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(res, `0.value.selector.matchLabels.replacedlabelgroup\.io`).String())
		require.Equal(t, "v", gjson.GetBytes(res, `0.value.template.metadata.labels.replacedlabelgroup\.io`).String())
		require.Equal(t, "a", gjson.GetBytes(res, `0.value.template.metadata.annotations.replacedanno\.io`).String())
		require.Equal(t, "v", gjson.GetBytes(res, `0.value.template.spec.nodeSelector.replacedlabelgroup\.io`).String())
		require.Equal(t, "replacedlabelgroup.io", gjson.GetBytes(res, `0.value.template.spec.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution.0.topologyKey`).String())
	})

	t.Run("returns error from RenameMetadataPatch", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "labels")
		patch := []byte(`{"metadata":{"labels":{"labelgroup.io":"v"}}}`)
		_, err := RenameSpecTemplatePatch(rules, patch)
		require.Error(t, err)
	})

	t.Run("json patch ignores other paths", func(t *testing.T) {
		patch := []byte(`[{"op":"replace","path":"/nope","value":{}}]`)
		res, err := RenameSpecTemplatePatch(rules, patch)
		require.NoError(t, err)
		require.Equal(t, "/nope", gjson.GetBytes(res, "0.path").String())
	})
}

func TestRewriteSpecTemplateLabelsAnno_ErrorBranches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	base := []byte(`{"spec":{"selector":{"matchLabels":{"labelgroup.io":"v"}},"template":{"metadata":{"labels":{"labelgroup.io":"v"},"annotations":{"annogroup.io":"a"}},"spec":{"nodeSelector":{"labelgroup.io":"v"},"affinity":{"podAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":[{"labelSelector":{"matchLabels":{"labelgroup.io":"v"}},"topologyKey":"labelgroup.io"}]}}}}}}`)

	t.Run("template metadata labels", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "template.metadata.labels")
		_, err := RewriteSpecTemplateLabelsAnno(rules, base, "spec", Rename)
		require.Error(t, err)
	})

	t.Run("selector matchLabels", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "selector.matchLabels")
		_, err := RewriteSpecTemplateLabelsAnno(rules, base, "spec", Rename)
		require.Error(t, err)
	})

	t.Run("nodeSelector", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "template.spec.nodeSelector")
		_, err := RewriteSpecTemplateLabelsAnno(rules, base, "spec", Rename)
		require.Error(t, err)
	})

	t.Run("affinity", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "topologyKey")
		_, err := RewriteSpecTemplateLabelsAnno(rules, base, "spec", Rename)
		require.Error(t, err)
	})
}
