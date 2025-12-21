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

package meta

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestMetaOptions(t *testing.T) {
	cm := &corev1.ConfigMap{}

	WithName[*corev1.ConfigMap]("cm")(cm)
	if cm.Name != "cm" {
		t.Fatalf("expected name set")
	}

	WithNamespace[*corev1.ConfigMap]("ns")(cm)
	if cm.Namespace != "ns" {
		t.Fatalf("expected namespace set")
	}

	WithGenerateName[*corev1.ConfigMap]("gen-")(cm)
	if cm.GenerateName != "gen-" {
		t.Fatalf("expected generateName set")
	}

	WithLabels[*corev1.ConfigMap](map[string]string{"a": "b"})(cm)
	if cm.Labels["a"] != "b" {
		t.Fatalf("expected labels set")
	}

	cm.Labels = nil
	WithLabel[*corev1.ConfigMap]("k", "v")(cm)
	if cm.Labels["k"] != "v" {
		t.Fatalf("expected label to be added when labels nil")
	}

	WithAnnotations[*corev1.ConfigMap](map[string]string{"a": "b"})(cm)
	if cm.Annotations["a"] != "b" {
		t.Fatalf("expected annotations set")
	}

	cm.Annotations = nil
	WithAnnotation[*corev1.ConfigMap]("k", "v")(cm)
	if cm.Annotations["k"] != "v" {
		t.Fatalf("expected annotation to be added when annotations nil")
	}

	WithFinalizer[*corev1.ConfigMap]("example.com/finalizer")(cm)
	if len(cm.Finalizers) != 1 || cm.Finalizers[0] != "example.com/finalizer" {
		t.Fatalf("expected finalizer to be added, got %#v", cm.Finalizers)
	}

	WithFinalizer[*corev1.ConfigMap]("example.com/finalizer")(cm)
	if len(cm.Finalizers) != 1 {
		t.Fatalf("expected finalizer to not be duplicated, got %#v", cm.Finalizers)
	}
}

