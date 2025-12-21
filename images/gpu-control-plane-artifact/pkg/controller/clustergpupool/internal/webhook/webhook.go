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

package webhook

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

type ClusterGPUPoolValidator struct {
	log      logr.Logger
	client   client.Client
	handlers []contracts.AdmissionHandler
}

func NewClusterGPUPoolValidator(log logr.Logger, c client.Client, handlers []contracts.AdmissionHandler) *ClusterGPUPoolValidator {
	return &ClusterGPUPoolValidator{
		log:      log.WithName("clustergpupool-webhook"),
		client:   c,
		handlers: handlers,
	}
}

func (v *ClusterGPUPoolValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (cradmission.Warnings, error) {
	clusterPool, ok := obj.(*v1alpha1.ClusterGPUPool)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterGPUPool but got a %T", obj)
	}

	if err := validateClusterPoolNameUnique(ctx, v.client, clusterPool); err != nil {
		return nil, err
	}

	pool := clusterPoolAsGPUPool(clusterPool)
	candidate := pool.DeepCopy()
	for _, h := range v.handlers {
		if _, err := h.SyncPool(ctx, candidate); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (v *ClusterGPUPoolValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (cradmission.Warnings, error) {
	oldClusterPool, ok := oldObj.(*v1alpha1.ClusterGPUPool)
	if !ok {
		return nil, fmt.Errorf("expected an old ClusterGPUPool but got a %T", oldObj)
	}
	newClusterPool, ok := newObj.(*v1alpha1.ClusterGPUPool)
	if !ok {
		return nil, fmt.Errorf("expected a new ClusterGPUPool but got a %T", newObj)
	}

	if !immutableEqual(clusterPoolAsGPUPool(oldClusterPool), clusterPoolAsGPUPool(newClusterPool)) {
		return nil, fmt.Errorf("immutable fields of ClusterGPUPool cannot be changed")
	}

	if err := validateClusterPoolNameUnique(ctx, v.client, newClusterPool); err != nil {
		return nil, err
	}

	pool := clusterPoolAsGPUPool(newClusterPool)
	candidate := pool.DeepCopy()
	for _, h := range v.handlers {
		if _, err := h.SyncPool(ctx, candidate); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (v *ClusterGPUPoolValidator) ValidateDelete(_ context.Context, _ runtime.Object) (cradmission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

type ClusterGPUPoolDefaulter struct {
	log      logr.Logger
	handlers []contracts.AdmissionHandler
}

func NewClusterGPUPoolDefaulter(log logr.Logger, handlers []contracts.AdmissionHandler) *ClusterGPUPoolDefaulter {
	return &ClusterGPUPoolDefaulter{
		log:      log.WithName("clustergpupool-webhook"),
		handlers: handlers,
	}
}

func (d *ClusterGPUPoolDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	clusterPool, ok := obj.(*v1alpha1.ClusterGPUPool)
	if !ok {
		return fmt.Errorf("expected a ClusterGPUPool but got a %T", obj)
	}

	pool := clusterPoolAsGPUPool(clusterPool)
	for _, h := range d.handlers {
		if _, err := h.SyncPool(ctx, pool); err != nil {
			return err
		}
	}

	clusterPool.Spec = pool.Spec
	clusterPool.Annotations = pool.Annotations
	clusterPool.Labels = pool.Labels
	return nil
}

var _ cradmission.CustomValidator = (*ClusterGPUPoolValidator)(nil)
var _ cradmission.CustomDefaulter = (*ClusterGPUPoolDefaulter)(nil)

func clusterPoolAsGPUPool(pool *v1alpha1.ClusterGPUPool) *v1alpha1.GPUPool {
	if pool == nil {
		return nil
	}
	out := &v1alpha1.GPUPool{
		TypeMeta:   pool.TypeMeta,
		ObjectMeta: pool.ObjectMeta,
		Spec:       pool.Spec,
		Status:     pool.Status,
	}
	if out.Kind == "" {
		out.Kind = "ClusterGPUPool"
	}
	return out
}

// immutableEqual checks that immutable parts of the pool spec were not changed.
func immutableEqual(old, cur *v1alpha1.GPUPool) bool {
	return reflect.DeepEqual(immutableView(old), immutableView(cur))
}

type immutableSpec struct {
	Provider       string
	Backend        string
	Resource       v1alpha1.GPUPoolResourceSpec
	DeviceSelector *v1alpha1.GPUPoolDeviceSelector
	NodeSelector   *metav1.LabelSelector
	Scheduling     v1alpha1.GPUPoolSchedulingSpec
}

func immutableView(p *v1alpha1.GPUPool) immutableSpec {
	return immutableSpec{
		Provider:       p.Spec.Provider,
		Backend:        p.Spec.Backend,
		Resource:       p.Spec.Resource,
		DeviceSelector: p.Spec.DeviceSelector,
		NodeSelector:   p.Spec.NodeSelector,
		Scheduling:     p.Spec.Scheduling,
	}
}

func validateClusterPoolNameUnique(ctx context.Context, c client.Client, pool *v1alpha1.ClusterGPUPool) error {
	if pool == nil {
		return nil
	}
	if c == nil {
		return fmt.Errorf("webhook client is not configured")
	}

	name := strings.TrimSpace(pool.Name)
	if name == "" {
		return nil
	}

	list := &v1alpha1.GPUPoolList{}
	if err := c.List(ctx, list); err != nil {
		return fmt.Errorf("list GPUPools: %w", err)
	}

	var namespaces []string
	for _, item := range list.Items {
		if item.Name != name {
			continue
		}
		namespaces = append(namespaces, item.Namespace)
	}
	if len(namespaces) == 0 {
		return nil
	}

	sort.Strings(namespaces)
	return fmt.Errorf("ClusterGPUPool name %q conflicts with existing GPUPool in namespaces: %s", name, strings.Join(namespaces, ", "))
}

