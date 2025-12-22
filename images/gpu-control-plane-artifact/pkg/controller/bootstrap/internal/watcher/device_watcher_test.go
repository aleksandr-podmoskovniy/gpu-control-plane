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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestDevicePredicates(t *testing.T) {
	p := devicePredicates()

	if !p.Create(event.TypedCreateEvent[*v1alpha1.GPUDevice]{Object: nil}) {
		t.Fatalf("expected create predicate to always trigger")
	}

	if !p.Delete(event.TypedDeleteEvent[*v1alpha1.GPUDevice]{Object: nil}) {
		t.Fatalf("expected delete predicate to always trigger")
	}

	if p.Generic(event.TypedGenericEvent[*v1alpha1.GPUDevice]{Object: &v1alpha1.GPUDevice{}}) {
		t.Fatalf("expected generic predicate to be ignored")
	}

	if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUDevice]{ObjectOld: nil, ObjectNew: &v1alpha1.GPUDevice{}}) {
		t.Fatalf("expected update predicate to pass through nil old")
	}
	if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUDevice]{ObjectOld: &v1alpha1.GPUDevice{}, ObjectNew: nil}) {
		t.Fatalf("expected update predicate to pass through nil new")
	}

	base := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev"},
		Status: v1alpha1.GPUDeviceStatus{
			State:    v1alpha1.GPUDeviceStateAssigned,
			NodeName: "node",
			Managed:  true,
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}

	same := base.DeepCopy()
	if p.Update(event.TypedUpdateEvent[*v1alpha1.GPUDevice]{ObjectOld: base, ObjectNew: same}) {
		t.Fatalf("expected update predicate to ignore unchanged device")
	}
	changed := base.DeepCopy()
	changed.Status.Managed = false
	if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUDevice]{ObjectOld: base, ObjectNew: changed}) {
		t.Fatalf("expected update predicate to trigger on changes")
	}
}

func TestDeviceChangedDetectsRelevantFields(t *testing.T) {
	base := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "dev"},
		Status: v1alpha1.GPUDeviceStatus{
			State:    v1alpha1.GPUDeviceStateAssigned,
			NodeName: "node",
			Managed:  true,
			PoolRef:  &v1alpha1.GPUPoolReference{Name: "pool"},
		},
	}

	same := base.DeepCopy()
	if deviceChanged(base, same) {
		t.Fatalf("expected no change for identical device")
	}

	changed := base.DeepCopy()
	changed.Status.State = v1alpha1.GPUDeviceStateReady
	if !deviceChanged(base, changed) {
		t.Fatalf("expected state change to be detected")
	}

	changed = base.DeepCopy()
	changed.Status.NodeName = "node-2"
	if !deviceChanged(base, changed) {
		t.Fatalf("expected nodeName change to be detected")
	}

	changed = base.DeepCopy()
	changed.Status.Managed = false
	if !deviceChanged(base, changed) {
		t.Fatalf("expected managed change to be detected")
	}

	changed = base.DeepCopy()
	changed.Status.PoolRef = nil
	if !deviceChanged(base, changed) {
		t.Fatalf("expected poolRef removal to be detected")
	}

	changed = base.DeepCopy()
	changed.Status.PoolRef.Name = "pool-2"
	if !deviceChanged(base, changed) {
		t.Fatalf("expected poolRef name change to be detected")
	}

	changed = base.DeepCopy()
	changed.Status.PoolRef.Namespace = "ns"
	if !deviceChanged(base, changed) {
		t.Fatalf("expected poolRef namespace change to be detected")
	}
}
