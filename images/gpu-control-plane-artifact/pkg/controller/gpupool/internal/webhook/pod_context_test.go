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

package webhook

import (
	"context"
	"testing"

	admv1 "k8s.io/api/admission/v1"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestEffectiveNamespaceBranches(t *testing.T) {
	t.Run("explicit", func(t *testing.T) {
		ctx := context.Background()
		if got := effectiveNamespace(ctx, "gpu-team"); got != "gpu-team" {
			t.Fatalf("expected explicit namespace, got %q", got)
		}
	})

	t.Run("from-request", func(t *testing.T) {
		req := cradmission.Request{AdmissionRequest: admv1.AdmissionRequest{Namespace: "from-ctx"}}
		ctx := cradmission.NewContextWithRequest(context.Background(), req)
		if got := effectiveNamespace(ctx, "  "); got != "from-ctx" {
			t.Fatalf("expected namespace from admission request, got %q", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		ctx := context.Background()
		if got := effectiveNamespace(ctx, ""); got != "" {
			t.Fatalf("expected empty namespace, got %q", got)
		}
	})
}

