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
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonannotations "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/annotations"
)

type GPUDeviceFilter struct {
	assignmentAnnotation string
}

func NewGPUDeviceFilter(assignmentAnnotation string) GPUDeviceFilter {
	return GPUDeviceFilter{assignmentAnnotation: assignmentAnnotation}
}

func (f GPUDeviceFilter) Predicates() predicate.TypedPredicate[*v1alpha1.GPUDevice] {
	return predicate.TypedFuncs[*v1alpha1.GPUDevice]{
		CreateFunc: func(e event.TypedCreateEvent[*v1alpha1.GPUDevice]) bool {
			dev := e.Object
			return dev != nil && (strings.TrimSpace(dev.Annotations[f.assignmentAnnotation]) != "" || poolRefValidForAssignment(dev.Status.PoolRef, f.assignmentAnnotation))
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha1.GPUDevice]) bool {
			oldDev := e.ObjectOld
			newDev := e.ObjectNew
			if oldDev == nil || newDev == nil {
				return true
			}
			return gpuDeviceChanged(oldDev, newDev, f.assignmentAnnotation)
		},
		DeleteFunc:  func(event.TypedDeleteEvent[*v1alpha1.GPUDevice]) bool { return true },
		GenericFunc: func(event.TypedGenericEvent[*v1alpha1.GPUDevice]) bool { return false },
	}
}

func gpuDeviceChanged(oldDev, newDev *v1alpha1.GPUDevice, assignmentAnnotation string) bool {
	if strings.TrimSpace(oldDev.Annotations[assignmentAnnotation]) != strings.TrimSpace(newDev.Annotations[assignmentAnnotation]) {
		return true
	}
	if oldDev.Status.State != newDev.Status.State || oldDev.Status.NodeName != newDev.Status.NodeName {
		return true
	}
	if oldDev.Status.Hardware.UUID != newDev.Status.Hardware.UUID {
		return true
	}
	if !equality.Semantic.DeepEqual(oldDev.Status.Hardware.MIG, newDev.Status.Hardware.MIG) {
		return true
	}
	oldRef := normalizedPoolRef(oldDev.Status.PoolRef, assignmentAnnotation)
	newRef := normalizedPoolRef(newDev.Status.PoolRef, assignmentAnnotation)
	if (oldRef == nil) != (newRef == nil) {
		return true
	}
	if oldRef != nil && newRef != nil {
		if oldRef.Name != newRef.Name || oldRef.Namespace != newRef.Namespace {
			return true
		}
	}
	return false
}

func normalizedPoolRef(ref *v1alpha1.GPUPoolReference, assignmentAnnotation string) *v1alpha1.GPUPoolReference {
	if !poolRefValidForAssignment(ref, assignmentAnnotation) {
		return nil
	}
	return ref
}

func poolRefValidForAssignment(ref *v1alpha1.GPUPoolReference, assignmentAnnotation string) bool {
	if ref == nil || ref.Name == "" {
		return false
	}
	switch assignmentAnnotation {
	case commonannotations.GPUDeviceAssignment:
		return ref.Namespace != ""
	case commonannotations.ClusterGPUDeviceAssignment:
		return strings.TrimSpace(ref.Namespace) == ""
	default:
		return true
	}
}
