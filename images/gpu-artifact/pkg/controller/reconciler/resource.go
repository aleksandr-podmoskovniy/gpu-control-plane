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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/object"
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
