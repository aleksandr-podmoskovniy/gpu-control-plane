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

func TestResourceValidator(t *testing.T) {
	validate := Resource()

	tests := []struct {
		name    string
		spec    *v1alpha1.GPUPoolSpec
		wantErr bool
	}{
		{
			name:    "empty-unit",
			spec:    &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: ""}},
			wantErr: true,
		},
		{
			name: "valid-card",
			spec: &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 2}},
		},
		{
			name: "valid-mig",
			spec: &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb", SlicesPerUnit: 1}},
		},
		{
			name:    "missing-mig-profile",
			spec:    &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG"}},
			wantErr: true,
		},
		{
			name:    "card-with-mig-profile",
			spec:    &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", MIGProfile: "1g.10gb", SlicesPerUnit: 1}},
			wantErr: true,
		},
		{
			name:    "invalid-mig-format",
			spec:    &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "bad", SlicesPerUnit: 1}},
			wantErr: true,
		},
		{
			name:    "unsupported-unit",
			spec:    &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Other", SlicesPerUnit: 1}},
			wantErr: true,
		},
		{
			name:    "slices-too-low",
			spec:    &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 0}},
			wantErr: true,
		},
		{
			name:    "slices-too-high",
			spec:    &v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 65}},
			wantErr: true,
		},
		{
			name:    "dra-with-mig",
			spec:    &v1alpha1.GPUPoolSpec{Backend: "DRA", Resource: v1alpha1.GPUPoolResourceSpec{Unit: "MIG", MIGProfile: "1g.10gb", SlicesPerUnit: 1}},
			wantErr: true,
		},
		{
			name:    "dra-with-slices",
			spec:    &v1alpha1.GPUPoolSpec{Backend: "DRA", Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 2}},
			wantErr: true,
		},
		{
			name: "dra-valid",
			spec: &v1alpha1.GPUPoolSpec{Backend: "DRA", Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card", SlicesPerUnit: 1}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(tc.spec)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
