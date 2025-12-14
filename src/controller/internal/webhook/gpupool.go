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
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	admv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

type gpupoolValidator struct {
	log      logr.Logger
	decoder  cradmission.Decoder
	handlers []contracts.AdmissionHandler
	client   client.Client
}

func newGPUPoolValidator(log logr.Logger, decoder cradmission.Decoder, handlers []contracts.AdmissionHandler, c client.Client) *gpupoolValidator {
	return &gpupoolValidator{
		log:      log.WithName("gpupool-validator"),
		decoder:  decoder,
		handlers: handlers,
		client:   c,
	}
}

func (v *gpupoolValidator) Handle(ctx context.Context, req cradmission.Request) cradmission.Response {
	pool := &v1alpha1.GPUPool{}
	if err := v.decoder.Decode(req, pool); err != nil {
		return cradmission.Errored(http.StatusUnprocessableEntity, err)
	}

	if req.Operation == admv1.Update && len(req.OldObject.Raw) > 0 {
		old := &v1alpha1.GPUPool{}
		if err := v.decoder.DecodeRaw(req.OldObject, old); err != nil {
			return cradmission.Errored(http.StatusUnprocessableEntity, err)
		}
		if !immutableEqual(old, pool) {
			return cradmission.Denied("immutable fields of GPUPool cannot be changed")
		}
	}

	if err := validateNamespacedPoolNameUnique(ctx, v.client, pool, req.Namespace); err != nil {
		return cradmission.Denied(err.Error())
	}

	candidate := pool.DeepCopy()
	for _, h := range v.handlers {
		if _, err := h.SyncPool(ctx, candidate); err != nil {
			return cradmission.Denied(err.Error())
		}
	}

	return cradmission.Allowed("validation passed")
}

func (v *gpupoolValidator) GVK() schema.GroupVersionKind {
	return v1alpha1.GroupVersion.WithKind("GPUPool")
}

type gpupoolDefaulter struct {
	log      logr.Logger
	decoder  cradmission.Decoder
	handlers []contracts.AdmissionHandler
}

func newGPUPoolDefaulter(log logr.Logger, decoder cradmission.Decoder, handlers []contracts.AdmissionHandler) *gpupoolDefaulter {
	return &gpupoolDefaulter{
		log:      log.WithName("gpupool-defaulter"),
		decoder:  decoder,
		handlers: handlers,
	}
}

func (d *gpupoolDefaulter) Handle(ctx context.Context, req cradmission.Request) cradmission.Response {
	pool := &v1alpha1.GPUPool{}
	if err := d.decoder.Decode(req, pool); err != nil {
		return cradmission.Errored(http.StatusUnprocessableEntity, err)
	}

	for _, h := range d.handlers {
		if _, err := h.SyncPool(ctx, pool); err != nil {
			return cradmission.Denied(err.Error())
		}
	}

	originalRaw := req.Object.Raw
	mutatedRaw, err := jsonMarshal(pool)
	if err != nil {
		return cradmission.Errored(http.StatusInternalServerError, err)
	}
	return cradmission.PatchResponseFromRaw(originalRaw, mutatedRaw)
}

func (d *gpupoolDefaulter) GVK() schema.GroupVersionKind {
	return v1alpha1.GroupVersion.WithKind("GPUPool")
}

// Ensure interfaces are satisfied.
var _ cradmission.Handler = &gpupoolValidator{}
var _ cradmission.Handler = &gpupoolDefaulter{}

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
