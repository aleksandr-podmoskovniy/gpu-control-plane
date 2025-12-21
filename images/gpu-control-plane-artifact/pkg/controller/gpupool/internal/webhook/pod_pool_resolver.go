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
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
)

func resolvePoolByRequest(ctx context.Context, c client.Client, req poolRequest, namespace string) (*v1alpha1.GPUPool, error) {
	if c == nil {
		return nil, fmt.Errorf("GPUPool %q: webhook client is not configured", req.name)
	}

	switch req.keyPrefix {
	case clusterPoolResourcePrefix:
		cluster := &v1alpha1.ClusterGPUPool{}
		cluster, err := commonobject.FetchObject(ctx, types.NamespacedName{Name: req.name}, c, cluster)
		if err != nil {
			return nil, err
		}
		if cluster == nil {
			return nil, fmt.Errorf("ClusterGPUPool %q not found", req.name)
		}
		return &v1alpha1.GPUPool{
			TypeMeta:   cluster.TypeMeta,
			ObjectMeta: cluster.ObjectMeta,
			Spec:       cluster.Spec,
			Status:     cluster.Status,
		}, nil
	case localPoolResourcePrefix:
		if strings.TrimSpace(namespace) == "" {
			return nil, fmt.Errorf("GPUPool %q: pod namespace is empty", req.name)
		}
		pool := &v1alpha1.GPUPool{}
		pool, err := commonobject.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: req.name}, c, pool)
		if err != nil {
			return nil, err
		}
		if pool == nil {
			return nil, fmt.Errorf("GPUPool %q not found in namespace %q", req.name, namespace)
		}
		return pool, nil
	default:
		return nil, fmt.Errorf("unknown pool resource prefix %q", req.keyPrefix)
	}
}
