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
	"sort"
	"strings"

	"github.com/go-logr/logr"
	admv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

type clusterGPUPoolValidator struct {
	log      logr.Logger
	decoder  cradmission.Decoder
	handlers []contracts.AdmissionHandler
	client   client.Client
}

func newClusterGPUPoolValidator(log logr.Logger, decoder cradmission.Decoder, handlers []contracts.AdmissionHandler, c client.Client) *clusterGPUPoolValidator {
	return &clusterGPUPoolValidator{
		log:      log.WithName("clustergpupool-validator"),
		decoder:  decoder,
		handlers: handlers,
		client:   c,
	}
}

func (v *clusterGPUPoolValidator) Handle(ctx context.Context, req cradmission.Request) cradmission.Response {
	clusterPool := &v1alpha1.ClusterGPUPool{}
	if err := v.decoder.Decode(req, clusterPool); err != nil {
		return cradmission.Errored(http.StatusUnprocessableEntity, err)
	}

	if req.Operation == admv1.Update && len(req.OldObject.Raw) > 0 {
		old := &v1alpha1.ClusterGPUPool{}
		if err := v.decoder.DecodeRaw(req.OldObject, old); err != nil {
			return cradmission.Errored(http.StatusUnprocessableEntity, err)
		}
		if !immutableEqual(clusterPoolAsGPUPool(old), clusterPoolAsGPUPool(clusterPool)) {
			return cradmission.Denied("immutable fields of ClusterGPUPool cannot be changed")
		}
	}

	if err := validateClusterPoolNameUnique(ctx, v.client, clusterPool); err != nil {
		return cradmission.Denied(err.Error())
	}

	pool := clusterPoolAsGPUPool(clusterPool)
	candidate := pool.DeepCopy()
	for _, h := range v.handlers {
		if _, err := h.SyncPool(ctx, candidate); err != nil {
			return cradmission.Denied(err.Error())
		}
	}

	return cradmission.Allowed("validation passed")
}

func (v *clusterGPUPoolValidator) GVK() schema.GroupVersionKind {
	return v1alpha1.GroupVersion.WithKind("ClusterGPUPool")
}

type clusterGPUPoolDefaulter struct {
	log      logr.Logger
	decoder  cradmission.Decoder
	handlers []contracts.AdmissionHandler
}

func newClusterGPUPoolDefaulter(log logr.Logger, decoder cradmission.Decoder, handlers []contracts.AdmissionHandler) *clusterGPUPoolDefaulter {
	return &clusterGPUPoolDefaulter{
		log:      log.WithName("clustergpupool-defaulter"),
		decoder:  decoder,
		handlers: handlers,
	}
}

func (d *clusterGPUPoolDefaulter) Handle(ctx context.Context, req cradmission.Request) cradmission.Response {
	clusterPool := &v1alpha1.ClusterGPUPool{}
	if err := d.decoder.Decode(req, clusterPool); err != nil {
		return cradmission.Errored(http.StatusUnprocessableEntity, err)
	}

	pool := clusterPoolAsGPUPool(clusterPool)
	for _, h := range d.handlers {
		if _, err := h.SyncPool(ctx, pool); err != nil {
			return cradmission.Denied(err.Error())
		}
	}

	clusterPool.Spec = pool.Spec
	clusterPool.Annotations = pool.Annotations
	clusterPool.Labels = pool.Labels

	originalRaw := req.Object.Raw
	mutatedRaw, err := jsonMarshal(clusterPool)
	if err != nil {
		return cradmission.Errored(http.StatusInternalServerError, err)
	}
	return cradmission.PatchResponseFromRaw(originalRaw, mutatedRaw)
}

func (d *clusterGPUPoolDefaulter) GVK() schema.GroupVersionKind {
	return v1alpha1.GroupVersion.WithKind("ClusterGPUPool")
}

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

var _ cradmission.Handler = &clusterGPUPoolValidator{}
var _ cradmission.Handler = &clusterGPUPoolDefaulter{}

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
