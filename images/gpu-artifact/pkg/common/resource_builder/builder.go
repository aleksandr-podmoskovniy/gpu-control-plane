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

package resource_builder //nolint:stylecheck,nolintlint

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ResourceBuilderOptions carries metadata about the target resource.
type ResourceBuilderOptions struct {
	ResourceExists bool
}

// ResourceBuilder helps constructing a Kubernetes object with common metadata.
type ResourceBuilder[T client.Object] struct {
	ResourceBuilderOptions
	Resource T
}

// NewResourceBuilder creates a new builder for a Kubernetes object.
func NewResourceBuilder[T client.Object](resource T, opts ResourceBuilderOptions) ResourceBuilder[T] {
	return ResourceBuilder[T]{
		ResourceBuilderOptions: opts,
		Resource:               resource,
	}
}

// SetOwnerRef sets the controller owner reference on the resource.
func (b *ResourceBuilder[T]) SetOwnerRef(obj metav1.Object, gvk schema.GroupVersionKind) {
	SetOwnerRef(b.Resource, *metav1.NewControllerRef(obj, gvk))
}

// AddAnnotation adds or updates a single annotation.
func (b *ResourceBuilder[T]) AddAnnotation(annotation, value string) {
	anns := b.Resource.GetAnnotations()
	if anns == nil {
		anns = make(map[string]string)
	}
	anns[annotation] = value
	b.Resource.SetAnnotations(anns)
}

// AddFinalizer adds a finalizer to the resource.
func (b *ResourceBuilder[T]) AddFinalizer(finalizer string) {
	controllerutil.AddFinalizer(b.Resource, finalizer)
}

// GetResource returns the underlying object.
func (b *ResourceBuilder[T]) GetResource() T {
	return b.Resource
}

// IsResourceExists returns whether the resource already exists in the cluster.
func (b *ResourceBuilder[T]) IsResourceExists() bool {
	return b.ResourceExists
}

// SetOwnerRef ensures the owner reference is present and returns true if it changed.
func SetOwnerRef(obj metav1.Object, ref metav1.OwnerReference) bool {
	refs := obj.GetOwnerReferences()
	for _, existing := range refs {
		if existing.Name == ref.Name {
			return false
		}
	}
	obj.SetOwnerReferences(append(refs, ref))
	return true
}
