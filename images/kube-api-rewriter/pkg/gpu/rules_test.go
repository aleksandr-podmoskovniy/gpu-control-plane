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

import "testing"

func TestGPURewriteRulesInit(t *testing.T) {
	t.Helper()

	GPURewriteRules.Init()
	if GPURewriteRules.LabelsRewriter() == nil ||
		GPURewriteRules.AnnotationsRewriter() == nil ||
		GPURewriteRules.FinalizersRewriter() == nil {
		t.Fatalf("metadata rewriters must be initialized even without custom rules")
	}

	if len(GPURewriteRules.Excludes) != 0 {
		t.Fatalf("expected no excludes for native GPU resources")
	}
}
