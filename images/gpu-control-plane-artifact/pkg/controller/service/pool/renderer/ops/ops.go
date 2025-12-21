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

package ops

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
)

func CreateOrUpdate(ctx context.Context, c client.Client, obj client.Object, pool *v1alpha1.GPUPool) error {
	desired := obj.DeepCopyObject().(client.Object)

	switch want := desired.(type) {
	case *corev1.ConfigMap:
		current := &corev1.ConfigMap{}
		current, err := commonobject.FetchObject(ctx, client.ObjectKeyFromObject(want), c, current)
		if err != nil {
			return err
		}
		if current == nil {
			addOwner(want, pool)
			return c.Create(ctx, want)
		}
		hadOwner := hasOwner(current, pool)
		addOwner(want, pool)
		addOwner(current, pool)
		if hadOwner && configMapEqual(current, want) {
			return nil
		}
		current.Labels = want.Labels
		current.Annotations = want.Annotations
		current.Data = want.Data
		current.BinaryData = want.BinaryData
		return c.Update(ctx, current)
	case *appsv1.DaemonSet:
		current := &appsv1.DaemonSet{}
		current, err := commonobject.FetchObject(ctx, client.ObjectKeyFromObject(want), c, current)
		if err != nil {
			return err
		}
		if current == nil {
			addOwner(want, pool)
			return c.Create(ctx, want)
		}
		hadOwner := hasOwner(current, pool)
		addOwner(want, pool)
		addOwner(current, pool)
		if hadOwner && daemonSetEqual(current, want) {
			return nil
		}
		current.Labels = want.Labels
		current.Annotations = want.Annotations
		current.Spec = want.Spec
		return c.Update(ctx, current)
	default:
		return fmt.Errorf("unsupported object type %T", obj)
	}
}

func addOwner(obj client.Object, pool *v1alpha1.GPUPool) {
	// Namespaced GPUPool cannot own resources in a different namespace; rely on explicit cleanup for those.
	if pool.Namespace != "" && obj.GetNamespace() != pool.Namespace {
		return
	}

	kind := pool.Kind
	if kind == "" {
		if pool.Namespace == "" {
			kind = "ClusterGPUPool"
		} else {
			kind = "GPUPool"
		}
	}
	owner := metav1.OwnerReference{
		APIVersion: v1alpha1.GroupVersion.String(),
		Kind:       kind,
		Name:       pool.Name,
		UID:        pool.UID,
		Controller: ptr.To(true),
	}
	refs := obj.GetOwnerReferences()
	for _, ref := range refs {
		if ref.APIVersion == owner.APIVersion && ref.Kind == owner.Kind && ref.Name == owner.Name {
			return
		}
	}
	obj.SetOwnerReferences(append(refs, owner))
}

func hasOwner(obj client.Object, pool *v1alpha1.GPUPool) bool {
	kind := pool.Kind
	if kind == "" {
		if pool.Namespace == "" {
			kind = "ClusterGPUPool"
		} else {
			kind = "GPUPool"
		}
	}
	for _, ref := range obj.GetOwnerReferences() {
		if ref.APIVersion == v1alpha1.GroupVersion.String() && ref.Kind == kind && ref.Name == pool.Name {
			return true
		}
	}
	return false
}

func configMapEqual(current, desired *corev1.ConfigMap) bool {
	return apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) &&
		apiequality.Semantic.DeepEqual(current.Annotations, desired.Annotations) &&
		apiequality.Semantic.DeepEqual(current.Data, desired.Data) &&
		apiequality.Semantic.DeepEqual(current.BinaryData, desired.BinaryData) &&
		apiequality.Semantic.DeepEqual(current.OwnerReferences, desired.OwnerReferences)
}

func daemonSetEqual(current, desired *appsv1.DaemonSet) bool {
	return apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) &&
		apiequality.Semantic.DeepEqual(current.Annotations, desired.Annotations) &&
		apiequality.Semantic.DeepEqual(current.Spec, desired.Spec) &&
		apiequality.Semantic.DeepEqual(current.OwnerReferences, desired.OwnerReferences)
}
