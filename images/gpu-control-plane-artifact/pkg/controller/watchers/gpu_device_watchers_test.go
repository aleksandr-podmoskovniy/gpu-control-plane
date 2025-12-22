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

package watchers

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonannotations "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/annotations"
)

func TestGPUDevicePredicates(t *testing.T) {
	tests := []struct {
		name                 string
		assignmentAnnotation string
	}{
		{name: "GPUPool", assignmentAnnotation: commonannotations.GPUDeviceAssignment},
		{name: "ClusterGPUPool", assignmentAnnotation: commonannotations.ClusterGPUDeviceAssignment},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewGPUDeviceFilter(tt.assignmentAnnotation).Predicates()
			poolRef := poolRefForAssignment(tt.assignmentAnnotation)

			if p.Create(event.TypedCreateEvent[*v1alpha1.GPUDevice]{Object: nil}) {
				t.Fatalf("expected create predicate to ignore nil device")
			}
			if p.Create(event.TypedCreateEvent[*v1alpha1.GPUDevice]{Object: &v1alpha1.GPUDevice{}}) {
				t.Fatalf("expected create predicate to ignore devices without pool assignment")
			}
			if !p.Create(event.TypedCreateEvent[*v1alpha1.GPUDevice]{Object: &v1alpha1.GPUDevice{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{tt.assignmentAnnotation: "pool"}}}}) {
				t.Fatalf("expected create predicate to trigger on assignment annotation")
			}
			if !p.Create(event.TypedCreateEvent[*v1alpha1.GPUDevice]{Object: &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{PoolRef: poolRef}}}) {
				t.Fatalf("expected create predicate to trigger on poolRef")
			}
			if tt.assignmentAnnotation == commonannotations.GPUDeviceAssignment {
				if p.Create(event.TypedCreateEvent[*v1alpha1.GPUDevice]{Object: &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "pool"}}}}) {
					t.Fatalf("expected create predicate to ignore unqualified poolRef")
				}
			} else {
				if p.Create(event.TypedCreateEvent[*v1alpha1.GPUDevice]{Object: &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{PoolRef: &v1alpha1.GPUPoolReference{Name: "pool", Namespace: "ns"}}}}) {
					t.Fatalf("expected create predicate to ignore namespaced poolRef for cluster pool")
				}
			}

			if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUDevice]{ObjectOld: nil, ObjectNew: &v1alpha1.GPUDevice{}}) {
				t.Fatalf("expected update predicate to pass through nil old")
			}
			if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUDevice]{ObjectOld: &v1alpha1.GPUDevice{}, ObjectNew: nil}) {
				t.Fatalf("expected update predicate to pass through nil new")
			}

			base := &v1alpha1.GPUDevice{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{tt.assignmentAnnotation: "pool"}},
				Status: v1alpha1.GPUDeviceStatus{
					State:    v1alpha1.GPUDeviceStateAssigned,
					NodeName: "node",
					Hardware: v1alpha1.GPUDeviceHardware{
						UUID: "uuid-1",
						MIG:  v1alpha1.GPUMIGConfig{Capable: true, Strategy: v1alpha1.GPUMIGStrategySingle},
					},
					PoolRef: poolRef,
				},
			}

			same := base.DeepCopy()
			if p.Update(event.TypedUpdateEvent[*v1alpha1.GPUDevice]{ObjectOld: base, ObjectNew: same}) {
				t.Fatalf("expected update predicate to ignore unchanged device")
			}
			changed := base.DeepCopy()
			changed.Annotations[tt.assignmentAnnotation] = "other"
			if !p.Update(event.TypedUpdateEvent[*v1alpha1.GPUDevice]{ObjectOld: base, ObjectNew: changed}) {
				t.Fatalf("expected update predicate to trigger on changes")
			}

			if !p.Delete(event.TypedDeleteEvent[*v1alpha1.GPUDevice]{Object: &v1alpha1.GPUDevice{}}) {
				t.Fatalf("expected delete predicate to trigger")
			}
			if p.Generic(event.TypedGenericEvent[*v1alpha1.GPUDevice]{Object: &v1alpha1.GPUDevice{}}) {
				t.Fatalf("expected generic predicate to be ignored")
			}
		})
	}
}

func TestGPUDeviceChangedDetectsRelevantFields(t *testing.T) {
	tests := []struct {
		name                 string
		assignmentAnnotation string
	}{
		{name: "GPUPool", assignmentAnnotation: commonannotations.GPUDeviceAssignment},
		{name: "ClusterGPUPool", assignmentAnnotation: commonannotations.ClusterGPUDeviceAssignment},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			poolRef := poolRefForAssignment(tt.assignmentAnnotation)
			base := &v1alpha1.GPUDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "dev",
					Annotations: map[string]string{tt.assignmentAnnotation: "pool"},
				},
				Status: v1alpha1.GPUDeviceStatus{
					State:    v1alpha1.GPUDeviceStateAssigned,
					NodeName: "node",
					Hardware: v1alpha1.GPUDeviceHardware{
						UUID: "uuid-1",
						MIG:  v1alpha1.GPUMIGConfig{Capable: true, Strategy: v1alpha1.GPUMIGStrategySingle},
					},
					PoolRef: poolRef,
				},
			}

			same := base.DeepCopy()
			if gpuDeviceChanged(base, same, tt.assignmentAnnotation) {
				t.Fatalf("expected no change for identical device")
			}

			changed := base.DeepCopy()
			changed.Annotations[tt.assignmentAnnotation] = "other"
			if !gpuDeviceChanged(base, changed, tt.assignmentAnnotation) {
				t.Fatalf("expected annotation change to be detected")
			}

			changed = base.DeepCopy()
			changed.Status.State = v1alpha1.GPUDeviceStateReady
			if !gpuDeviceChanged(base, changed, tt.assignmentAnnotation) {
				t.Fatalf("expected state change to be detected")
			}

			changed = base.DeepCopy()
			changed.Status.NodeName = "node-2"
			if !gpuDeviceChanged(base, changed, tt.assignmentAnnotation) {
				t.Fatalf("expected nodeName change to be detected")
			}

			changed = base.DeepCopy()
			changed.Status.Hardware.UUID = "uuid-2"
			if !gpuDeviceChanged(base, changed, tt.assignmentAnnotation) {
				t.Fatalf("expected UUID change to be detected")
			}

			changed = base.DeepCopy()
			changed.Status.Hardware.MIG.Strategy = v1alpha1.GPUMIGStrategyMixed
			if !gpuDeviceChanged(base, changed, tt.assignmentAnnotation) {
				t.Fatalf("expected MIG change to be detected")
			}

			changed = base.DeepCopy()
			changed.Status.PoolRef = nil
			if !gpuDeviceChanged(base, changed, tt.assignmentAnnotation) {
				t.Fatalf("expected poolRef removal to be detected")
			}

			changed = base.DeepCopy()
			changed.Status.PoolRef.Name = "pool-2"
			if !gpuDeviceChanged(base, changed, tt.assignmentAnnotation) {
				t.Fatalf("expected poolRef name change to be detected")
			}

			changed = base.DeepCopy()
			if tt.assignmentAnnotation == commonannotations.GPUDeviceAssignment {
				changed.Status.PoolRef.Namespace = "other"
			} else {
				changed.Status.PoolRef.Namespace = "ns"
			}
			if !gpuDeviceChanged(base, changed, tt.assignmentAnnotation) {
				t.Fatalf("expected poolRef namespace change to be detected")
			}
		})
	}
}

func poolRefForAssignment(assignmentAnnotation string) *v1alpha1.GPUPoolReference {
	ref := &v1alpha1.GPUPoolReference{Name: "pool"}
	if assignmentAnnotation == commonannotations.GPUDeviceAssignment {
		ref.Namespace = "ns"
	}
	return ref
}
