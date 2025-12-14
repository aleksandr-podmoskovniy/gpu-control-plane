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
	"errors"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

var errEmptyPodAdmissionRequest = errors.New("empty pod admission request")

func decodePodRequest(req cradmission.Request) (*corev1.Pod, []byte, error) {
	switch {
	case len(req.Object.Raw) > 0:
		pod := &corev1.Pod{}
		if err := json.Unmarshal(req.Object.Raw, pod); err != nil {
			return nil, nil, err
		}
		return pod, req.Object.Raw, nil
	case req.Object.Object != nil:
		pod, ok := req.Object.Object.(*corev1.Pod)
		if !ok {
			return nil, nil, fmt.Errorf("request object is not a Pod")
		}
		raw, err := jsonMarshal(pod)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal pod admission object: %w", err)
		}
		return pod, raw, nil
	default:
		return nil, nil, errEmptyPodAdmissionRequest
	}
}

func selectSinglePool(pod *corev1.Pod) (poolRequest, bool, error) {
	pools := collectPools(pod)
	if len(pools) == 0 {
		return poolRequest{}, false, nil
	}
	if len(pools) == 1 {
		for _, p := range pools {
			return p, true, nil
		}
	}

	names := make([]string, 0, len(pools))
	for _, p := range pools {
		names = append(names, p.keyPrefix+p.name)
	}
	sort.Strings(names)
	return poolRequest{}, false, fmt.Errorf("multiple GPU pools requested: %v", names)
}

const gpuEnabledLabelKey = "gpu.deckhouse.io/enabled"

func requireGPUEnabledNamespace(ctx context.Context, c client.Client, namespace string) error {
	if c == nil {
		return nil
	}
	if strings.TrimSpace(namespace) == "" {
		return fmt.Errorf("pod namespace is empty")
	}

	ns := &corev1.Namespace{}
	if err := c.Get(ctx, client.ObjectKey{Name: namespace}, ns); err != nil {
		return fmt.Errorf("namespace %q not found: %v", namespace, err)
	}
	if ns.Labels[gpuEnabledLabelKey] != "true" {
		return fmt.Errorf("namespace %q is not enabled for GPU (label %s=true is required)", namespace, gpuEnabledLabelKey)
	}
	return nil
}

func resolvePoolByRequest(ctx context.Context, c client.Client, req poolRequest, namespace string) (*v1alpha1.GPUPool, error) {
	if c == nil {
		return nil, fmt.Errorf("GPUPool %q: webhook client is not configured", req.name)
	}

	switch req.keyPrefix {
	case clusterPoolResourcePrefix:
		cluster := &v1alpha1.ClusterGPUPool{}
		if err := c.Get(ctx, client.ObjectKey{Name: req.name}, cluster); err == nil {
			return &v1alpha1.GPUPool{
				TypeMeta:   cluster.TypeMeta,
				ObjectMeta: cluster.ObjectMeta,
				Spec:       cluster.Spec,
				Status:     cluster.Status,
			}, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("ClusterGPUPool %q not found", req.name)
	case localPoolResourcePrefix:
		if strings.TrimSpace(namespace) == "" {
			return nil, fmt.Errorf("GPUPool %q: pod namespace is empty", req.name)
		}
		pool := &v1alpha1.GPUPool{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: req.name}, pool); err == nil {
			return pool, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("GPUPool %q not found in namespace %q", req.name, namespace)
	default:
		return nil, fmt.Errorf("unknown pool resource prefix %q", req.keyPrefix)
	}
}
