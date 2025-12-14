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
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRenameWebhookConfigurationPatch_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("merge patch restores rules", func(t *testing.T) {
		patch := []byte(`{"webhooks":[{"rules":[{"apiGroups":["prefixed.resources.group.io"],"resources":["prefixedsomeresources"],"verbs":["get"]}]}]}`)
		res, err := RenameWebhookConfigurationPatch(rules, patch)
		require.NoError(t, err)

		require.Equal(t, "original.group.io", gjson.GetBytes(res, "webhooks.0.rules.0.apiGroups.0").String())
		require.Equal(t, "someresources", gjson.GetBytes(res, "webhooks.0.rules.0.resources.0").String())
	})

	t.Run("json patch rewrites /webhooks", func(t *testing.T) {
		patch := []byte(`[{"op":"replace","path":"/webhooks","value":[{"rules":[{"apiGroups":["original.group.io"],"resources":["someresources"],"verbs":["get"]}]}]}]`)
		res, err := RenameWebhookConfigurationPatch(rules, patch)
		require.NoError(t, err)
		require.Equal(t, "prefixed.resources.group.io", gjson.GetBytes(res, "0.value.0.rules.0.apiGroups.0").String())
		require.Equal(t, "prefixedsomeresources", gjson.GetBytes(res, "0.value.0.rules.0.resources.0").String())
	})

	t.Run("returns error from RenameMetadataPatch", func(t *testing.T) {
		origSetBytes := sjsonSetBytes
		t.Cleanup(func() { sjsonSetBytes = origSetBytes })
		sjsonSetBytes = func(_ []byte, _ string, _ interface{}) ([]byte, error) {
			return nil, errors.New("boom")
		}

		patch := []byte(`{"metadata":{"labels":{"labelgroup.io":"v"}}}`)
		_, err := RenameWebhookConfigurationPatch(rules, patch)
		require.Error(t, err)
	})

	t.Run("json patch ignores other paths", func(t *testing.T) {
		patch := []byte(`[{"op":"replace","path":"/nope","value":{}}]`)
		res, err := RenameWebhookConfigurationPatch(rules, patch)
		require.NoError(t, err)
		require.Equal(t, "/nope", gjson.GetBytes(res, "0.path").String())
	})
}
