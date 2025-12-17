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
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

type GPUPoolValidator struct {
	log      logr.Logger
	client   client.Client
	handlers []contracts.AdmissionHandler
}

func NewGPUPoolValidator(log logr.Logger, c client.Client, handlers []contracts.AdmissionHandler) *GPUPoolValidator {
	return &GPUPoolValidator{
		log:      log.WithName("gpupool-webhook"),
		client:   c,
		handlers: handlers,
	}
}

func (v *GPUPoolValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (cradmission.Warnings, error) {
	pool, ok := obj.(*v1alpha1.GPUPool)
	if !ok {
		return nil, fmt.Errorf("expected a GPUPool but got a %T", obj)
	}

	admissionNamespace := pool.Namespace
	if admissionNamespace == "" {
		if req, err := cradmission.RequestFromContext(ctx); err == nil {
			admissionNamespace = req.Namespace
		}
	}

	if err := validateNamespacedPoolNameUnique(ctx, v.client, pool, admissionNamespace); err != nil {
		return nil, err
	}

	candidate := pool.DeepCopy()
	for _, h := range v.handlers {
		if _, err := h.SyncPool(ctx, candidate); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (v *GPUPoolValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (cradmission.Warnings, error) {
	oldPool, ok := oldObj.(*v1alpha1.GPUPool)
	if !ok {
		return nil, fmt.Errorf("expected an old GPUPool but got a %T", oldObj)
	}
	newPool, ok := newObj.(*v1alpha1.GPUPool)
	if !ok {
		return nil, fmt.Errorf("expected a new GPUPool but got a %T", newObj)
	}

	if !immutableEqual(oldPool, newPool) {
		return nil, fmt.Errorf("immutable fields of GPUPool cannot be changed")
	}

	admissionNamespace := newPool.Namespace
	if admissionNamespace == "" {
		if req, err := cradmission.RequestFromContext(ctx); err == nil {
			admissionNamespace = req.Namespace
		}
	}

	if err := validateNamespacedPoolNameUnique(ctx, v.client, newPool, admissionNamespace); err != nil {
		return nil, err
	}

	candidate := newPool.DeepCopy()
	for _, h := range v.handlers {
		if _, err := h.SyncPool(ctx, candidate); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (v *GPUPoolValidator) ValidateDelete(_ context.Context, _ runtime.Object) (cradmission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

type GPUPoolDefaulter struct {
	log      logr.Logger
	handlers []contracts.AdmissionHandler
}

func NewGPUPoolDefaulter(log logr.Logger, handlers []contracts.AdmissionHandler) *GPUPoolDefaulter {
	return &GPUPoolDefaulter{
		log:      log.WithName("gpupool-webhook"),
		handlers: handlers,
	}
}

func (d *GPUPoolDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	pool, ok := obj.(*v1alpha1.GPUPool)
	if !ok {
		return fmt.Errorf("expected a GPUPool but got a %T", obj)
	}

	for _, h := range d.handlers {
		if _, err := h.SyncPool(ctx, pool); err != nil {
			return err
		}
	}
	return nil
}

var _ cradmission.CustomValidator = (*GPUPoolValidator)(nil)
var _ cradmission.CustomDefaulter = (*GPUPoolDefaulter)(nil)

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

func validateNamespacedPoolNameUnique(ctx context.Context, c client.Client, pool *v1alpha1.GPUPool, admissionNamespace string) error {
	if pool == nil {
		return nil
	}
	if c == nil {
		return fmt.Errorf("webhook client is not configured")
	}

	ns := pool.Namespace
	if ns == "" {
		ns = admissionNamespace
	}

	name := strings.TrimSpace(pool.Name)
	if name == "" {
		return nil
	}

	clusterPool := &v1alpha1.ClusterGPUPool{}
	if err := c.Get(ctx, client.ObjectKey{Name: name}, clusterPool); err == nil {
		return fmt.Errorf("GPUPool name %q conflicts with existing ClusterGPUPool of the same name", name)
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("check ClusterGPUPool %q: %w", name, err)
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
		if item.Namespace == ns {
			continue
		}
		namespaces = append(namespaces, item.Namespace)
	}

	if len(namespaces) == 0 {
		return nil
	}

	sort.Strings(namespaces)
	return fmt.Errorf("GPUPool name %q must be unique cluster-wide (found in namespaces: %s)", name, strings.Join(namespaces, ", "))
}
