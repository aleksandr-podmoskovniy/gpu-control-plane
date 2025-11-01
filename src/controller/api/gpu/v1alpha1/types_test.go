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

package v1alpha1_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestAddToSchemeRegistersAllTypes(t *testing.T) {
	scheme := runtime.NewScheme()

	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme returned error: %v", err)
	}

	kinds := []string{
		"GPUDevice",
		"GPUDeviceList",
		"GPUNodeInventory",
		"GPUNodeInventoryList",
		"GPUPool",
		"GPUPoolList",
	}

	for _, kind := range kinds {
		gvk := v1alpha1.GroupVersion.WithKind(kind)
		if _, err := scheme.New(gvk); err != nil {
			t.Fatalf("expected kind %s to be registered in scheme: %v", kind, err)
		}
	}

	if _, err := scheme.New(metav1.SchemeGroupVersion.WithKind("Status")); err != nil {
		t.Fatalf("expected metav1 types to remain in scheme: %v", err)
	}
}

func TestGroupVersionMetadata(t *testing.T) {
	if v1alpha1.GroupVersion.Group != "gpu.deckhouse.io" {
		t.Fatalf("unexpected group: %s", v1alpha1.GroupVersion.Group)
	}
	if v1alpha1.GroupVersion.Version != "v1alpha1" {
		t.Fatalf("unexpected version: %s", v1alpha1.GroupVersion.Version)
	}
}
