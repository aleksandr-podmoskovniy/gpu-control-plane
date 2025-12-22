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

package watchers

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonannotations "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/annotations"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
)

type GPUPoolGPUDeviceEnqueuer struct {
	log logr.Logger
	cl  client.Client
}

func NewGPUPoolGPUDeviceEnqueuer(log logr.Logger, cl client.Client) *GPUPoolGPUDeviceEnqueuer {
	return &GPUPoolGPUDeviceEnqueuer{log: log, cl: cl}
}

func (e *GPUPoolGPUDeviceEnqueuer) EnqueueRequests(ctx context.Context, dev *v1alpha1.GPUDevice) []reconcile.Request {
	if dev == nil {
		return nil
	}

	targetPools := map[string]struct{}{}
	reqSet := map[types.NamespacedName]struct{}{}

	if ref := dev.Status.PoolRef; ref != nil {
		if ref.Name != "" && ref.Namespace != "" {
			reqSet[types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}] = struct{}{}
		}
	}

	if ann := strings.TrimSpace(dev.Annotations[commonannotations.GPUDeviceAssignment]); ann != "" {
		targetPools[ann] = struct{}{}
	}

	if len(targetPools) == 0 || e.cl == nil {
		return requestsFromSet(reqSet)
	}

	for poolName := range targetPools {
		list := &v1alpha1.GPUPoolList{}
		if err := e.cl.List(ctx, list, client.MatchingFields{indexer.GPUPoolNameField: poolName}); err != nil {
			if e.log.GetSink() != nil {
				e.log.Error(err, "list GPUPool by name to map device event", "device", dev.Name, "pool", poolName)
			}
			continue
		}
		for i := range list.Items {
			pool := list.Items[i]
			reqSet[types.NamespacedName{Namespace: pool.Namespace, Name: pool.Name}] = struct{}{}
		}
	}

	return requestsFromSet(reqSet)
}

type ClusterGPUPoolGPUDeviceEnqueuer struct{}

func NewClusterGPUPoolGPUDeviceEnqueuer() *ClusterGPUPoolGPUDeviceEnqueuer {
	return &ClusterGPUPoolGPUDeviceEnqueuer{}
}

func (e *ClusterGPUPoolGPUDeviceEnqueuer) EnqueueRequests(_ context.Context, dev *v1alpha1.GPUDevice) []reconcile.Request {
	if dev == nil {
		return nil
	}

	targets := map[types.NamespacedName]struct{}{}
	if ref := dev.Status.PoolRef; ref != nil {
		if ref.Name != "" && strings.TrimSpace(ref.Namespace) == "" {
			targets[types.NamespacedName{Name: ref.Name}] = struct{}{}
		}
	}
	if ann := strings.TrimSpace(dev.Annotations[commonannotations.ClusterGPUDeviceAssignment]); ann != "" {
		targets[types.NamespacedName{Name: ann}] = struct{}{}
	}

	return requestsFromSet(targets)
}

func requestsFromSet(reqSet map[types.NamespacedName]struct{}) []reconcile.Request {
	if len(reqSet) == 0 {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(reqSet))
	for nn := range reqSet {
		reqs = append(reqs, reconcile.Request{NamespacedName: nn})
	}
	return reqs
}
