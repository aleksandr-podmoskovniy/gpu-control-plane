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
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAPIEndpoint_Branches(t *testing.T) {
	{
		u, err := url.Parse("https://example.com/foo")
		require.NoError(t, err)
		ep := ParseAPIEndpoint(u)
		require.NotNil(t, ep)
		require.True(t, ep.IsUnknown)
		require.Equal(t, "/foo", ep.Path())
	}

	{
		u, err := url.Parse("https://example.com/api/v1/namespaces/ns/pods")
		require.NoError(t, err)
		ep := ParseAPIEndpoint(u)
		require.NotNil(t, ep)
		require.True(t, ep.IsCore)
		require.Equal(t, "ns", ep.Namespace)
		require.Equal(t, "pods", ep.ResourceType)
		require.Equal(t, "", ep.Name)
		require.Equal(t, "", ep.Subresource)
	}

	{
		u, err := url.Parse("https://example.com/api/v1/namespaces/ns/status")
		require.NoError(t, err)
		ep := ParseAPIEndpoint(u)
		require.NotNil(t, ep)
		require.True(t, ep.IsCore)
		require.Equal(t, "", ep.Namespace)
		require.Equal(t, "namespaces", ep.ResourceType)
		require.Equal(t, "ns", ep.Name)
		require.Equal(t, "status", ep.Subresource)
	}

	{
		u, err := url.Parse("https://example.com/api/v1/pods/p1/status/e1/e2/e3")
		require.NoError(t, err)
		ep := ParseAPIEndpoint(u)
		require.NotNil(t, ep)
		require.True(t, ep.IsCore)
		require.Equal(t, []string{"e3"}, ep.Remainder)
		require.Equal(t, "pods", ep.ResourceType)
		require.Equal(t, "e1", ep.Name)
		require.Equal(t, "e2", ep.Subresource)
	}

	{
		u, err := url.Parse("https://example.com/apis/original.group.io/v1/namespaces/ns/someresources?watch=true")
		require.NoError(t, err)
		ep := ParseAPIEndpoint(u)
		require.NotNil(t, ep)
		require.True(t, ep.IsWatch)
		require.Equal(t, "ns", ep.Namespace)
		require.Equal(t, "someresources", ep.ResourceType)
		require.Equal(t, "/apis/original.group.io/v1/namespaces/ns/someresources", ep.Path())
	}

	{
		u, err := url.Parse("https://example.com/apis/original.group.io/v1/someresources/s1/status/e1/e2/e3")
		require.NoError(t, err)
		ep := ParseAPIEndpoint(u)
		require.NotNil(t, ep)
		require.Equal(t, []string{"e3"}, ep.Remainder)
		require.Equal(t, "/apis/original.group.io/v1/someresources/e1/e2/e3", ep.Path())
	}
}

func TestShift_Empty(t *testing.T) {
	var items []string
	first, last := Shift(&items)
	require.Equal(t, "", first)
	require.True(t, last)
}
