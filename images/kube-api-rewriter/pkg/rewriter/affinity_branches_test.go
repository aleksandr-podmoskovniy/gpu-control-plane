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

func TestRewriteAffinity_Rename_FullStructure(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	obj := []byte(`{
  "spec": {
    "affinity": {
      "nodeAffinity": {
        "requiredDuringSchedulingIgnoredDuringExecution": {
          "nodeSelectorTerms": [
            {
              "matchLabels": [
                {"key":"labelgroup.io","operator":"In","values":["labelValueToRename","other"]}
              ],
              "matchExpressions": [
                {"key":"labelgroup.io","operator":"In","values":["labelValueToRename"]}
              ]
            }
          ]
        },
        "preferredDuringSchedulingIgnoredDuringExecution": [
          {
            "weight": 1,
            "preference": {
              "matchLabels": [
                {"key":"labelgroup.io","operator":"In","values":["labelValueToRename"]}
              ],
              "matchExpressions": [
                {"key":"labelgroup.io","operator":"In","values":["labelValueToRename"]}
              ]
            }
          }
        ]
      },
      "podAffinity": {
        "requiredDuringSchedulingIgnoredDuringExecution": [
          {
            "labelSelector": {
              "matchLabels": {"labelgroup.io":"v"},
              "matchExpressions": [
                {"key":"labelgroup.io","operator":"In","values":["labelValueToRename"]}
              ]
            },
            "topologyKey": "labelgroup.io",
            "namespaceSelector": {
              "matchLabels": {"labelgroup.io":"v"},
              "matchExpressions": [
                {"key":"labelgroup.io","operator":"In","values":["labelValueToRename"]}
              ]
            },
            "matchLabelKeys": ["labelgroup.io"],
            "mismatchLabelKeys": ["component.labelgroup.io/labelkey"]
          }
        ],
        "preferredDuringSchedulingIgnoredDuringExecution": [
          {
            "weight": 1,
            "podAffinityTerm": {
              "labelSelector": {
                "matchLabels": {"labelgroup.io":"v"}
              },
              "topologyKey": "labelgroup.io"
            }
          }
        ]
      },
      "podAntiAffinity": {
        "requiredDuringSchedulingIgnoredDuringExecution": [
          {
            "labelSelector": {
              "matchLabels": {"labelgroup.io":"v"}
            },
            "topologyKey": "labelgroup.io"
          }
        ]
      }
    }
  }
}`)

	res, err := RewriteAffinity(rules, obj, "spec.affinity", Rename)
	require.NoError(t, err)

	require.Equal(t, "replacedlabelgroup.io", gjson.GetBytes(res, "spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms.0.matchExpressions.0.key").String())
	require.Equal(t, "renamedLabelValue", gjson.GetBytes(res, "spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms.0.matchExpressions.0.values.0").String())

	require.Equal(t, "v", gjson.GetBytes(res, `spec.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution.0.labelSelector.matchLabels.replacedlabelgroup\.io`).String())
	require.Equal(t, "replacedlabelgroup.io", gjson.GetBytes(res, "spec.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution.0.topologyKey").String())
	require.Equal(t, "replacedlabelgroup.io", gjson.GetBytes(res, "spec.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution.0.matchLabelKeys.0").String())
	require.Equal(t, "component.replacedlabelgroup.io/labelkey", gjson.GetBytes(res, "spec.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution.0.mismatchLabelKeys.0").String())
}

func TestAffinityRewriters_ErrorBranches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("rewriteSelectorRequirement returns error on key set", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "key")
		_, err := rewriteSelectorRequirement(rules, []byte(`{"key":"labelgroup.io","values":["labelValueToRename"]}`), Rename)
		require.Error(t, err)
	})

	t.Run("rewriteNodeSelectorTerm returns error", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "key")
		_, err := rewriteNodeSelectorTerm(rules, []byte(`{"matchLabels":[{"key":"labelgroup.io","values":["labelValueToRename"]}]}`), Rename)
		require.Error(t, err)
	})

	t.Run("rewriteNodeAffinity returns error", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "key")
		_, err := rewriteNodeAffinity(rules, []byte(`{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"labelgroup.io","values":["labelValueToRename"]}]}]}}`), Rename)
		require.Error(t, err)
	})

	t.Run("RewriteAffinity returns error from nodeAffinity", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "key")
		obj := []byte(`{"spec":{"affinity":{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"labelgroup.io","values":["labelValueToRename"]}]}]}}}}}`)
		_, err := RewriteAffinity(rules, obj, "spec.affinity", Rename)
		require.Error(t, err)
	})

	t.Run("RewriteAffinity returns error from podAffinity", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "topologyKey")
		obj := []byte(`{"spec":{"affinity":{"podAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":[{"topologyKey":"labelgroup.io","labelSelector":{"matchLabels":{"labelgroup.io":"v"}}}]}}}}`)
		_, err := RewriteAffinity(rules, obj, "spec.affinity", Rename)
		require.Error(t, err)
	})

	t.Run("rewritePodAffinity returns error from requiredDuringSchedulingIgnoredDuringExecution", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "topologyKey")
		_, err := rewritePodAffinity(rules, []byte(`{"requiredDuringSchedulingIgnoredDuringExecution":[{"topologyKey":"labelgroup.io","labelSelector":{"matchLabels":{"labelgroup.io":"v"}}}]}`), Rename)
		require.Error(t, err)
	})

	t.Run("rewritePodAffinityTerm errors at each stage", func(t *testing.T) {
		base := []byte(`{
  "labelSelector":{"matchLabels":{"labelgroup.io":"v"}},
  "topologyKey":"labelgroup.io",
  "namespaceSelector":{"matchLabels":{"labelgroup.io":"v"}},
  "matchLabelKeys":["labelgroup.io"],
  "mismatchLabelKeys":["labelgroup.io"]
}`)

		t.Run("labelSelector", func(t *testing.T) {
			failSjsonSetBytesOnPath(t, "matchLabels")
			_, err := rewritePodAffinityTerm(rules, base, Rename)
			require.Error(t, err)
		})

		t.Run("topologyKey", func(t *testing.T) {
			failSjsonSetBytesOnPath(t, "topologyKey")
			_, err := rewritePodAffinityTerm(rules, base, Rename)
			require.Error(t, err)
		})

		t.Run("namespaceSelector", func(t *testing.T) {
			failSjsonSetRawBytesOnPath(t, "namespaceSelector")
			_, err := rewritePodAffinityTerm(rules, base, Rename)
			require.Error(t, err)
		})

		t.Run("matchLabelKeys", func(t *testing.T) {
			failSjsonSetBytesOnPath(t, "matchLabelKeys")
			_, err := rewritePodAffinityTerm(rules, base, Rename)
			require.Error(t, err)
		})

		t.Run("mismatchLabelKeys", func(t *testing.T) {
			failSjsonSetBytesOnPath(t, "mismatchLabelKeys")
			_, err := rewritePodAffinityTerm(rules, base, Rename)
			require.Error(t, err)
		})
	})

	t.Run("rewriteLabelSelector returns error from RewriteLabelsMap", func(t *testing.T) {
		failSjsonSetBytesOnPath(t, "matchLabels")
		_, err := rewriteLabelSelector(rules, []byte(`{"matchLabels":{"labelgroup.io":"v"}}`), Rename)
		require.Error(t, err)
	})
}
