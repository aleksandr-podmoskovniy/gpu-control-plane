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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
)

type GPUPoolValidatorPodEnqueuer struct {
	log logr.Logger
	cl  client.Client
}

func NewGPUPoolValidatorPodEnqueuer(log logr.Logger, cl client.Client) *GPUPoolValidatorPodEnqueuer {
	return &GPUPoolValidatorPodEnqueuer{log: log, cl: cl}
}

func (e *GPUPoolValidatorPodEnqueuer) EnqueueRequests(ctx context.Context, pod *corev1.Pod) []reconcile.Request {
	if !isValidatorPoolPod(pod) {
		return nil
	}
	poolName := strings.TrimSpace(pod.Labels["pool"])

	if e.cl == nil {
		return nil
	}

	list := &v1alpha1.GPUPoolList{}
	if err := e.cl.List(ctx, list, client.MatchingFields{indexer.GPUPoolNameField: poolName}); err != nil {
		if e.log.GetSink() != nil {
			e.log.Error(err, "list GPUPool by name to map validator pod event", "pod", pod.Name, "pool", poolName)
		}
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		pool := list.Items[i]
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: pool.Namespace, Name: pool.Name},
		})
	}
	return reqs
}

type ClusterGPUPoolValidatorPodEnqueuer struct{}

func NewClusterGPUPoolValidatorPodEnqueuer() *ClusterGPUPoolValidatorPodEnqueuer {
	return &ClusterGPUPoolValidatorPodEnqueuer{}
}

func (e *ClusterGPUPoolValidatorPodEnqueuer) EnqueueRequests(_ context.Context, pod *corev1.Pod) []reconcile.Request {
	if !isValidatorPoolPod(pod) {
		return nil
	}
	poolName := strings.TrimSpace(pod.Labels["pool"])
	if poolName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: poolName}}}
}
