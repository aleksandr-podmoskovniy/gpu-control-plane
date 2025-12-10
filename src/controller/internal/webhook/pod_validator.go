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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

// podValidator rejects pods requesting GPU resources when namespace/pool access is not allowed.
type podValidator struct {
	log     logr.Logger
	decoder cradmission.Decoder
	client  client.Client
}

func newPodValidator(log logr.Logger, decoder cradmission.Decoder, _ interface{}, c client.Client) *podValidator {
	return &podValidator{
		log:     log.WithName("pod-validator"),
		decoder: decoder,
		client:  c,
	}
}

func (v *podValidator) Handle(ctx context.Context, req cradmission.Request) cradmission.Response {
	pod := &corev1.Pod{}
	switch {
	case len(req.Object.Raw) > 0:
		if err := json.Unmarshal(req.Object.Raw, pod); err != nil {
			return cradmission.Errored(422, err)
		}
	case req.Object.Object != nil:
		if p, ok := req.Object.Object.(*corev1.Pod); ok {
			pod = p
		} else {
			return cradmission.Errored(422, fmt.Errorf("request object is not a Pod"))
		}
	default:
		return cradmission.Denied("empty pod admission request")
	}

	pools := collectPools(pod)
	if len(pools) == 0 {
		return cradmission.Allowed("no gpu pool requested")
	}

	if v.client != nil {
		ns := &corev1.Namespace{}
		if err := v.client.Get(ctx, client.ObjectKey{Name: pod.Namespace}, ns); err != nil {
			return cradmission.Denied(fmt.Sprintf("namespace %q not found: %v", pod.Namespace, err))
		}
		if strings.ToLower(ns.Labels["gpu.deckhouse.io/enabled"]) != "true" {
			return cradmission.Denied(fmt.Sprintf("namespace %q is not enabled for GPU (label gpu.deckhouse.io/enabled=true is required)", pod.Namespace))
		}
	}

	if len(pools) > 1 {
		names := make([]string, 0, len(pools))
		for _, p := range pools {
			names = append(names, p.keyPrefix+p.name)
		}
		return cradmission.Denied(fmt.Sprintf("multiple GPU pools requested: %v", names))
	}

	var poolRef poolRequest
	for _, p := range pools {
		poolRef = p
	}

	requested := requestedResources(pod, poolRef)
	if requested <= 0 {
		return cradmission.Allowed("no gpu pool requested")
	}

	if _, err := v.resolvePool(ctx, poolRef, pod.Namespace); err != nil {
		return cradmission.Denied(err.Error())
	}

	if v.client != nil {
		available, err := v.poolAvailable(ctx, poolRef, pod.Namespace)
		if err != nil {
			return cradmission.Denied(err.Error())
		}
		if requested > available {
			return cradmission.Denied(fmt.Sprintf("requested %d units of %s but only %d available", requested, poolRef.keyPrefix+poolRef.name, available))
		}
	}

	return cradmission.Allowed("gpu pod validated")
}

func (v *podValidator) GVK() schema.GroupVersionKind {
	return corev1.SchemeGroupVersion.WithKind("Pod")
}

func (v *podValidator) resolvePool(ctx context.Context, req poolRequest, ns string) (*v1alpha1.GPUPool, error) {
	if v.client == nil {
		return nil, fmt.Errorf("GPUPool %q: webhook client is not configured", req.name)
	}
	if ns == "" && !req.isCluster {
		return nil, fmt.Errorf("GPUPool %q: pod namespace is empty", req.name)
	}
	if req.isCluster {
		cluster := &v1alpha1.ClusterGPUPool{}
		if err := v.client.Get(ctx, client.ObjectKey{Name: req.name}, cluster); err == nil {
			return &v1alpha1.GPUPool{
				ObjectMeta: cluster.ObjectMeta,
				Spec:       cluster.Spec,
			}, nil
		}
		return nil, fmt.Errorf("ClusterGPUPool %q not found", req.name)
	}

	pool := &v1alpha1.GPUPool{}
	if err := v.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: req.name}, pool); err == nil {
		return pool, nil
	}
	return nil, fmt.Errorf("GPUPool %q not found in namespace %q", req.name, ns)
}

func (v *podValidator) poolAvailable(ctx context.Context, req poolRequest, ns string) (int64, error) {
	if v.client == nil {
		return 0, fmt.Errorf("webhook client is not configured")
	}
	if req.isCluster {
		obj := &v1alpha1.ClusterGPUPool{}
		if err := v.client.Get(ctx, client.ObjectKey{Name: req.name}, obj); err != nil {
			return 0, err
		}
		return int64(obj.Status.Capacity.Available), nil
	}
	obj := &v1alpha1.GPUPool{}
	if err := v.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: req.name}, obj); err != nil {
		return 0, err
	}
	return int64(obj.Status.Capacity.Available), nil
}

func requestedResources(pod *corev1.Pod, pool poolRequest) int64 {
	name := corev1.ResourceName(pool.keyPrefix + pool.name)
	var total int64
	acc := func(res corev1.ResourceList) {
		if q, ok := res[name]; ok {
			total += q.Value()
		}
	}
	for _, c := range pod.Spec.Containers {
		acc(c.Resources.Limits)
		if _, ok := c.Resources.Limits[name]; !ok {
			acc(c.Resources.Requests)
		}
	}
	for _, c := range pod.Spec.InitContainers {
		acc(c.Resources.Limits)
		if _, ok := c.Resources.Limits[name]; !ok {
			acc(c.Resources.Requests)
		}
	}
	return total
}
