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

package gpupool

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/event"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestPoolPredicates(t *testing.T) {
	pred := poolPredicates()

	if !pred.Create(event.CreateEvent{Object: &v1alpha1.GPUPool{}}) {
		t.Fatalf("expected create to pass through")
	}

	old := &v1alpha1.GPUPool{Spec: v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}}}
	newSame := old.DeepCopy()
	if pred.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: newSame}) {
		t.Fatalf("expected update with same spec to be filtered out")
	}

	newDiff := old.DeepCopy()
	newDiff.Spec.Resource.Unit = "MIG"
	if !pred.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: newDiff}) {
		t.Fatalf("expected update with spec change to trigger")
	}
	if !pred.Update(event.UpdateEvent{ObjectOld: nil, ObjectNew: newDiff}) {
		t.Fatalf("expected update with nil old to pass through")
	}
	if !pred.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: nil}) {
		t.Fatalf("expected update with nil new to pass through")
	}
	if !pred.Delete(event.DeleteEvent{Object: old}) {
		t.Fatalf("expected delete to trigger")
	}
	if pred.Generic(event.GenericEvent{}) {
		t.Fatalf("expected generic to be ignored")
	}
}
