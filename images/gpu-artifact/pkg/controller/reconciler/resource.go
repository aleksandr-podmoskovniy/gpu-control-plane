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

package reconciler

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/object"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/patch"
)

// ResourceObject describes a typed Kubernetes object with status.
type ResourceObject[T, ST any] interface {
	comparable
	client.Object
	DeepCopy() T
	GetObjectMeta() metav1.Object
}

// ObjectStatusGetter returns the status subresource from an object.
type ObjectStatusGetter[T, ST any] func(obj T) ST

// ObjectFactory creates a new empty object.
type ObjectFactory[T any] func() T

// Resource provides a typed wrapper around a Kubernetes object.
type Resource[T ResourceObject[T, ST], ST any] struct {
	name       types.NamespacedName
	currentObj T
	changedObj T
	emptyObj   T

	objFactory      ObjectFactory[T]
	objStatusGetter ObjectStatusGetter[T, ST]
	client          client.Client
}

// NewResource creates a new Resource wrapper.
func NewResource[T ResourceObject[T, ST], ST any](name types.NamespacedName, cl client.Client, objFactory ObjectFactory[T], objStatusGetter ObjectStatusGetter[T, ST]) *Resource[T, ST] {
	return &Resource[T, ST]{
		name:            name,
		client:          cl,
		objFactory:      objFactory,
		objStatusGetter: objStatusGetter,
	}
}

func (r *Resource[T, ST]) getObjStatus(obj T) (ret ST) {
	if obj != r.emptyObj {
		ret = r.objStatusGetter(obj)
	}
	return
}

// Name returns the namespaced name of the resource.
func (r *Resource[T, ST]) Name() types.NamespacedName {
	return r.name
}

// Fetch loads the current object and creates a writable copy.
func (r *Resource[T, ST]) Fetch(ctx context.Context) error {
	currentObj, err := object.FetchObject(ctx, r.name, r.client, r.objFactory())
	if err != nil {
		return err
	}

	r.currentObj = currentObj
	if r.IsEmpty() {
		r.changedObj = r.emptyObj
		return nil
	}

	r.changedObj = currentObj.DeepCopy()
	return nil
}

// IsEmpty returns true when the object does not exist.
func (r *Resource[T, ST]) IsEmpty() bool {
	return r.currentObj == r.emptyObj
}

// Current returns the fetched object.
func (r *Resource[T, ST]) Current() T {
	return r.currentObj
}

// Changed returns the mutable copy of the object.
func (r *Resource[T, ST]) Changed() T {
	return r.changedObj
}

// Update writes status changes and patches metadata changes.
func (r *Resource[T, ST]) Update(ctx context.Context) error {
	if r.IsEmpty() {
		return nil
	}

	if !reflect.DeepEqual(r.getObjStatus(r.currentObj), r.getObjStatus(r.changedObj)) {
		finalizers := r.changedObj.GetFinalizers()
		labels := r.changedObj.GetLabels()
		annotations := r.changedObj.GetAnnotations()
		if err := r.client.Status().Update(ctx, r.changedObj); err != nil {
			return fmt.Errorf("error updating status subresource: %w", err)
		}
		r.changedObj.SetFinalizers(finalizers)
		r.changedObj.SetLabels(labels)
		r.changedObj.SetAnnotations(annotations)
	}

	metadataPatch := patch.NewJSONPatch()

	if !slices.Equal(r.currentObj.GetFinalizers(), r.changedObj.GetFinalizers()) {
		metadataPatch.Append(r.jsonPatchOpsForFinalizers()...)
	}
	if !maps.Equal(r.currentObj.GetAnnotations(), r.changedObj.GetAnnotations()) {
		metadataPatch.Append(r.jsonPatchOpsForAnnotations()...)
	}
	if !maps.Equal(r.currentObj.GetLabels(), r.changedObj.GetLabels()) {
		metadataPatch.Append(r.jsonPatchOpsForLabels()...)
	}

	if metadataPatch.Len() == 0 {
		return nil
	}

	metadataPatchBytes, err := metadataPatch.Bytes()
	if err != nil {
		return err
	}
	jsonPatch := client.RawPatch(types.JSONPatchType, metadataPatchBytes)
	if err = r.client.Patch(ctx, r.changedObj, jsonPatch); err != nil {
		if r.changedObj.GetDeletionTimestamp() != nil && len(r.changedObj.GetFinalizers()) == 0 && kerrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("error patching metadata (%s): %w", string(metadataPatchBytes), err)
	}

	return nil
}

func (r *Resource[T, ST]) jsonPatchOpsForFinalizers() []patch.JSONPatchOperation {
	return []patch.JSONPatchOperation{
		patch.NewJSONPatchOperation(patch.PatchReplaceOp, "/metadata/finalizers", r.changedObj.GetFinalizers()),
	}
}

func (r *Resource[T, ST]) jsonPatchOpsForAnnotations() []patch.JSONPatchOperation {
	return []patch.JSONPatchOperation{
		patch.NewJSONPatchOperation(patch.PatchTestOp, "/metadata/annotations", r.currentObj.GetAnnotations()),
		patch.NewJSONPatchOperation(patch.PatchReplaceOp, "/metadata/annotations", r.changedObj.GetAnnotations()),
	}
}

func (r *Resource[T, ST]) jsonPatchOpsForLabels() []patch.JSONPatchOperation {
	return []patch.JSONPatchOperation{
		patch.NewJSONPatchOperation(patch.PatchTestOp, "/metadata/labels", r.currentObj.GetLabels()),
		patch.NewJSONPatchOperation(patch.PatchReplaceOp, "/metadata/labels", r.changedObj.GetLabels()),
	}
}
