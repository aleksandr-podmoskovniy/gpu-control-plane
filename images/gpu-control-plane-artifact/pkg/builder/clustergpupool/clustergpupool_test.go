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

package clustergpupool

import (
	"reflect"
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestNewEmpty(t *testing.T) {
	pool := NewEmpty("p")
	if pool.Name != "p" {
		t.Fatalf("unexpected name: %q", pool.Name)
	}
	if pool.Kind != "ClusterGPUPool" {
		t.Fatalf("unexpected kind: %q", pool.Kind)
	}
	if pool.APIVersion == "" {
		t.Fatalf("expected apiVersion to be set")
	}
}

func TestNewAppliesOptions(t *testing.T) {
	spec := v1alpha1.GPUPoolSpec{
		Provider: "Nvidia",
		Backend:  "DevicePlugin",
		Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"},
	}
	pool := New(
		WithName("p"),
		WithLabel("k", "v"),
		WithAnnotation("a", "b"),
		WithSpec(spec),
	)

	if pool.Name != "p" {
		t.Fatalf("unexpected metadata: %#v", pool.ObjectMeta)
	}
	if pool.Labels["k"] != "v" || pool.Annotations["a"] != "b" {
		t.Fatalf("expected labels/annotations to be set: %#v %#v", pool.Labels, pool.Annotations)
	}
	if !reflect.DeepEqual(pool.Spec, spec) {
		t.Fatalf("expected spec applied")
	}
}

func TestApplyOptionsNilPool(t *testing.T) {
	ApplyOptions(nil, []Option{WithName("p")})
}
