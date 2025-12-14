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
)

func TestRewriteGVK_Branches(t *testing.T) {
	rwr := createTestRewriter()
	rules := rwr.Rules

	t.Run("rename rewrites when rule exists", func(t *testing.T) {
		obj := []byte(`{"kind":"SomeResource","apiVersion":"original.group.io/v1"}`)
		res, err := RewriteAPIVersionAndKind(rules, obj, Rename)
		require.NoError(t, err)
		require.Contains(t, string(res), `"kind":"PrefixedSomeResource"`)
		require.Contains(t, string(res), `"apiVersion":"prefixed.resources.group.io/v1"`)
	})

	t.Run("rename returns as-is when no rule", func(t *testing.T) {
		obj := []byte(`{"kind":"NoSuchKind","apiVersion":"original.group.io/v1"}`)
		res, err := RewriteAPIVersionAndKind(rules, obj, Rename)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("restore returns as-is when group not renamed", func(t *testing.T) {
		obj := []byte(`{"kind":"PrefixedSomeResource","apiVersion":"original.group.io/v1"}`)
		res, err := RewriteAPIVersionAndKind(rules, obj, Restore)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("restore rewrites when rule exists", func(t *testing.T) {
		obj := []byte(`{"kind":"PrefixedSomeResource","apiVersion":"prefixed.resources.group.io/v1"}`)
		res, err := RewriteAPIVersionAndKind(rules, obj, Restore)
		require.NoError(t, err)
		require.Contains(t, string(res), `"kind":"SomeResource"`)
		require.Contains(t, string(res), `"apiVersion":"original.group.io/v1"`)
	})

	t.Run("restore returns as-is when restored kind has no rule", func(t *testing.T) {
		obj := []byte(`{"kind":"PrefixedNoSuchKind","apiVersion":"prefixed.resources.group.io/v1"}`)
		res, err := RewriteAPIVersionAndKind(rules, obj, Restore)
		require.NoError(t, err)
		require.Equal(t, string(obj), string(res))
	})

	t.Run("error from sjson.SetBytes is propagated", func(t *testing.T) {
		orig := sjsonSetBytes
		sjsonSetBytes = func([]byte, string, any) ([]byte, error) {
			return nil, errors.New("boom")
		}
		t.Cleanup(func() { sjsonSetBytes = orig })

		_, err := RewriteAPIVersionAndKind(rules, []byte(`{"kind":"SomeResource","apiVersion":"original.group.io/v1"}`), Rename)
		require.Error(t, err)
	})
}
