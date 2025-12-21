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

package validators

import (
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestProviderValidator(t *testing.T) {
	validate := Provider("Nvidia")
	if err := validate(&v1alpha1.GPUPoolSpec{Provider: "AMD"}); err == nil {
		t.Fatalf("expected unsupported provider error")
	}
	if err := validate(&v1alpha1.GPUPoolSpec{Provider: "Nvidia"}); err != nil {
		t.Fatalf("expected provider to be valid, got %v", err)
	}
}
