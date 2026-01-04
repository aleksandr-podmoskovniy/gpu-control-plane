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

package resource_builder

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestResourceBuilderMetadata(t *testing.T) {
	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "cm-1",
		},
	}
	owner := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "owner-1",
		},
	}
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}

	builder := NewResourceBuilder(obj, ResourceBuilderOptions{ResourceExists: true})
	if !builder.IsResourceExists() {
		t.Fatalf("expected ResourceExists=true")
	}

	builder.AddAnnotation("gpu.deckhouse.io/test", "true")
	if obj.GetAnnotations()["gpu.deckhouse.io/test"] != "true" {
		t.Fatalf("expected annotation to be set")
	}

	builder.AddFinalizer("gpu.deckhouse.io/finalizer")
	if len(obj.GetFinalizers()) != 1 {
		t.Fatalf("expected finalizer to be set")
	}

	builder.SetOwnerRef(owner, gvk)
	if len(obj.GetOwnerReferences()) != 1 {
		t.Fatalf("expected owner reference to be set")
	}

	if builder.GetResource() != obj {
		t.Fatalf("expected to return the original object")
	}

	ref := *metav1.NewControllerRef(owner, gvk)
	if SetOwnerRef(obj, ref) {
		t.Fatalf("expected SetOwnerRef to return false for existing owner")
	}
}
