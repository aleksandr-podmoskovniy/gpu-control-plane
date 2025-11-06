/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gpu

import (
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/rewriter"
)

func cloneRules(t *testing.T, src *rewriter.RewriteRules) *rewriter.RewriteRules {
	t.Helper()

	cloned := *src
	cloned.Rules = make(map[string]rewriter.APIGroupRule, len(src.Rules))
	for group, rule := range src.Rules {
		resourceRules := make(map[string]rewriter.ResourceRule, len(rule.ResourceRules))
		for resource, resRule := range rule.ResourceRules {
			resourceRules[resource] = resRule
		}
		cloned.Rules[group] = rewriter.APIGroupRule{
			GroupRule:     rule.GroupRule,
			ResourceRules: resourceRules,
		}
	}
	return &cloned
}

func TestGPURewriteRulesInit(t *testing.T) {
	rules := cloneRules(t, GPURewriteRules)
	rules.Init()

	groupRule, ok := rules.Rules[apiGroup]
	if !ok {
		t.Fatalf("expected %q present in rules", apiGroup)
	}
	if groupRule.GroupRule.Renamed != internalAPIGroup {
		t.Fatalf("expected renamed group %q, got %q", internalAPIGroup, groupRule.GroupRule.Renamed)
	}

	deviceRule, ok := groupRule.ResourceRules["gpudevices"]
	if !ok {
		t.Fatalf("gpudevices rule missing")
	}
	if deviceRule.Kind != "GPUDevice" {
		t.Fatalf("unexpected kind: %s", deviceRule.Kind)
	}
	if deviceRule.ListKind != "GPUDeviceList" {
		t.Fatalf("unexpected list kind: %s", deviceRule.ListKind)
	}
	if deviceRule.PreferredVersion != "v1alpha1" {
		t.Fatalf("unexpected preferred version: %s", deviceRule.PreferredVersion)
	}

	if rules.Rules[apiGroup].GroupRule.Group != apiGroup {
		t.Fatalf("original group should remain intact")
	}

	if rules.LabelsRewriter() == nil || rules.AnnotationsRewriter() == nil {
		t.Fatalf("expected metadata rewriters to be initialized")
	}
}

func TestGPURewriteRulesProvideCategories(t *testing.T) {
	rules := cloneRules(t, GPURewriteRules)
	rules.Init()

	groupRule := rules.Rules[apiGroup]
	for _, res := range []string{"gpudevices", "gpunodeinventories", "gpupools"} {
		if _, ok := groupRule.ResourceRules[res]; !ok {
			t.Fatalf("expected resource %s in rule set", res)
		}
		if len(groupRule.ResourceRules[res].Categories) == 0 {
			t.Fatalf("resource %s must expose categories for discovery", res)
		}
	}
}
