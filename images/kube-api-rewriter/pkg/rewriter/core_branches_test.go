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

func TestCoreRewriters_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("pod list", func(t *testing.T) {
		list := []byte(`{"kind":"PodList","items":[{"kind":"Pod","spec":{"nodeSelector":{"labelgroup.io":"v"}}}]}`)
		res, err := RewritePodOrList(rules, list, Rename)
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(res, `items.0.spec.nodeSelector.replacedlabelgroup\.io`).String())
	})

	t.Run("pod single with affinity", func(t *testing.T) {
		pod := []byte(`{"kind":"Pod","spec":{"nodeSelector":{"labelgroup.io":"v"},"affinity":{"podAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":[{"labelSelector":{"matchLabels":{"labelgroup.io":"v"}},"topologyKey":"labelgroup.io"}]}}}}`)
		res, err := RewritePodOrList(rules, pod, Rename)
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(res, `spec.nodeSelector.replacedlabelgroup\.io`).String())
		require.Equal(t, "replacedlabelgroup.io", gjson.GetBytes(res, `spec.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution.0.topologyKey`).String())
	})

	t.Run("pod returns error from RewriteLabelsMap", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "spec.nodeSelector")
		pod := []byte(`{"kind":"Pod","spec":{"nodeSelector":{"labelgroup.io":"v"}}}`)
		_, err := RewritePodOrList(rules, pod, Rename)
		require.Error(t, err)
	})

	t.Run("pvc list", func(t *testing.T) {
		list := []byte(`{"kind":"PersistentVolumeClaimList","items":[{"kind":"PersistentVolumeClaim","spec":{"dataSource":{"apiGroup":"original.group.io","kind":"SomeResource"},"dataSourceRef":{"apiGroup":"original.group.io","kind":"SomeResource"}}}]}`)
		res, err := RewritePVCOrList(rules, list, Rename)
		require.NoError(t, err)
		require.Equal(t, "prefixed.resources.group.io", gjson.GetBytes(res, "items.0.spec.dataSource.apiGroup").String())
		require.Equal(t, "PrefixedSomeResource", gjson.GetBytes(res, "items.0.spec.dataSource.kind").String())
	})

	t.Run("pvc returns error from TransformObject", func(t *testing.T) {
		failSjsonSetRawBytesOnPath(t, "spec.dataSource")
		obj := []byte(`{"kind":"PersistentVolumeClaim","spec":{"dataSource":{"apiGroup":"original.group.io","kind":"SomeResource"}}}`)
		_, err := RewritePVCOrList(rules, obj, Rename)
		require.Error(t, err)
	})

	t.Run("service patch merge passthrough", func(t *testing.T) {
		patch := []byte(`{"spec":{"selector":{"labelgroup.io":"v"}}}`)
		res, err := RenameServicePatch(rules, patch)
		require.NoError(t, err)
		require.Equal(t, string(patch), string(res))
	})

	t.Run("service patch json ignores other paths", func(t *testing.T) {
		patch := []byte(`[{"op":"replace","path":"/nope","value":{}}]`)
		res, err := RenameServicePatch(rules, patch)
		require.NoError(t, err)
		require.Equal(t, "/nope", gjson.GetBytes(res, "0.path").String())
	})

	t.Run("service patch json rewrites /spec", func(t *testing.T) {
		patch := []byte(`[{"op":"replace","path":"/spec","value":{"selector":{"labelgroup.io":"v"}}}]`)
		res, err := RenameServicePatch(rules, patch)
		require.NoError(t, err)
		require.Equal(t, "v", gjson.GetBytes(res, `0.value.selector.replacedlabelgroup\.io`).String())
	})

	t.Run("service patch returns error from RenameMetadataPatch", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "labels")
		patch := []byte(`{"metadata":{"labels":{"labelgroup.io":"v"}}}`)
		_, err := RenameServicePatch(rules, patch)
		require.Error(t, err)
	})
}
