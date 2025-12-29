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

package conditions

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestManagerAdd(t *testing.T) {
	mgr := NewManager([]metav1.Condition{{Type: "Ready"}})

	if added := mgr.Add(metav1.Condition{Type: "Healthy"}); !added {
		t.Fatalf("expected new condition to be added")
	}
	if got := len(mgr.Generate()); got != 2 {
		t.Fatalf("expected 2 conditions, got %d", got)
	}

	if added := mgr.Add(metav1.Condition{Type: "Ready"}); added {
		t.Fatalf("expected existing condition to be ignored")
	}
	if got := len(mgr.Generate()); got != 2 {
		t.Fatalf("expected 2 conditions after duplicate add, got %d", got)
	}
}
