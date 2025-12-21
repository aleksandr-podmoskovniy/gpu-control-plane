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

package watcher

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/event"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	gpupoolbuilder "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/builder/gpupool"
)

func TestPoolPredicates(t *testing.T) {
	pred := PoolPredicates()

	if !pred.Create(event.TypedCreateEvent[*v1alpha1.GPUPool]{Object: &v1alpha1.GPUPool{}}) {
		t.Fatalf("expected create to pass through")
	}

	old := gpupoolbuilder.New(gpupoolbuilder.WithSpec(v1alpha1.GPUPoolSpec{Resource: v1alpha1.GPUPoolResourceSpec{Unit: "Card"}}))
	newSame := old.DeepCopy()
	if pred.Update(event.TypedUpdateEvent[*v1alpha1.GPUPool]{ObjectOld: old, ObjectNew: newSame}) {
		t.Fatalf("expected update with same spec to be filtered out")
	}

	newDiff := old.DeepCopy()
	newDiff.Spec.Resource.Unit = "MIG"
	if !pred.Update(event.TypedUpdateEvent[*v1alpha1.GPUPool]{ObjectOld: old, ObjectNew: newDiff}) {
		t.Fatalf("expected update with spec change to trigger")
	}
	if !pred.Update(event.TypedUpdateEvent[*v1alpha1.GPUPool]{ObjectOld: nil, ObjectNew: newDiff}) {
		t.Fatalf("expected update with nil old to pass through")
	}
	if !pred.Update(event.TypedUpdateEvent[*v1alpha1.GPUPool]{ObjectOld: old, ObjectNew: nil}) {
		t.Fatalf("expected update with nil new to pass through")
	}
	if !pred.Delete(event.TypedDeleteEvent[*v1alpha1.GPUPool]{Object: old}) {
		t.Fatalf("expected delete to trigger")
	}
	if pred.Generic(event.TypedGenericEvent[*v1alpha1.GPUPool]{}) {
		t.Fatalf("expected generic to be ignored")
	}
}

