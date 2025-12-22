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

package names

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestResolveResourceNameUsesPoolNameAndStripsPrefix(t *testing.T) {
	pool := &v1alpha1.GPUPool{ObjectMeta: metav1.ObjectMeta{Name: "alpha"}}

	if got := ResolveResourceName(pool, ""); got != "alpha" {
		t.Fatalf("expected pool name fallback, got %q", got)
	}

	if got := ResolveResourceName(pool, " gpu.deckhouse.io/beta "); got != "beta" {
		t.Fatalf("expected prefix to be stripped, got %q", got)
	}
}
