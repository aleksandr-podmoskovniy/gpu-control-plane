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

package proxy

import (
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/rewriter"
)

func newSimpleRewriter(t *testing.T) *rewriter.RuleBasedRewriter {
	t.Helper()

	rules := &rewriter.RewriteRules{
		KindPrefix:         "Prefixed",
		ResourceTypePrefix: "prefixed",
		ShortNamePrefix:    "p",
		Categories:         []string{"prefixed"},
		Rules: map[string]rewriter.APIGroupRule{
			"original.group.io": {
				GroupRule: rewriter.GroupRule{
					Group:            "original.group.io",
					Versions:         []string{"v1"},
					PreferredVersion: "v1",
					Renamed:          "prefixed.resources.group.io",
				},
				ResourceRules: map[string]rewriter.ResourceRule{
					"someresources": {
						Kind:             "SomeResource",
						ListKind:         "SomeResourceList",
						Plural:           "someresources",
						Singular:         "someresource",
						Versions:         []string{"v1"},
						PreferredVersion: "v1",
						Categories:       []string{"all"},
						ShortNames:       []string{"sr"},
					},
				},
			},
		},
	}
	rules.Init()

	return &rewriter.RuleBasedRewriter{Rules: rules}
}
