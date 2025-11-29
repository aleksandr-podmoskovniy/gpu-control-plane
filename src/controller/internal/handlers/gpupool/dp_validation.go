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
	"os"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// DPValidationHandler tracks per-pool validator readiness per node and updates device states.
// PendingAssignment -> Assigned when validator pod Ready on device node.
type DPValidationHandler struct {
	log     logr.Logger
	client  client.Client
	ns      string
	podList func(ctx context.Context, cl client.Client, opts ...client.ListOption) (*corev1.PodList, error)
}

func NewDPValidationHandler(log logr.Logger, cl client.Client) *DPValidationHandler {
	ns := os.Getenv("POD_NAMESPACE")
	if ns == "" {
		ns = "d8-gpu-control-plane"
	}
	return &DPValidationHandler{
		log:    log,
		client: cl,
		ns:     ns,
		podList: func(ctx context.Context, cl client.Client, opts ...client.ListOption) (*corev1.PodList, error) {
			l := &corev1.PodList{}
			if err := cl.List(ctx, l, opts...); err != nil {
				return nil, err
			}
			return l, nil
		},
	}
}

func (h *DPValidationHandler) Name() string {
	return "dp-validation"
}

func (h *DPValidationHandler) HandlePool(ctx context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	if h.client == nil {
		return contracts.Result{}, nil
	}
	if pool.Status.Capacity.Total == 0 {
		return contracts.Result{}, nil
	}

	// collect validator-ready nodes for this pool
	pods := &corev1.PodList{}
	{
		l, err := h.podList(ctx, h.client,
			client.InNamespace(h.ns),
			client.MatchingLabels{"app": "nvidia-operator-validator", "pool": pool.Name},
		)
		if err != nil && !errors.IsNotFound(err) {
			return contracts.Result{}, err
		}
		if l != nil {
			pods = l
		}
	}
	readyNodes := map[string]bool{}
	for i := range pods.Items {
		p := &pods.Items[i]
		if p.Spec.NodeName == "" {
			continue
		}
		if isPodReady(p) {
			readyNodes[p.Spec.NodeName] = true
		}
	}

	devices := &v1alpha1.GPUDeviceList{}
	if err := h.client.List(ctx, devices); err != nil {
		return contracts.Result{}, err
	}

	changed := false
	for i := range devices.Items {
		dev := &devices.Items[i]
		if dev.Annotations[assignmentAnnotation] != pool.Name {
			continue
		}
		node := dev.Status.NodeName
		if node == "" {
			node = dev.Labels["kubernetes.io/hostname"]
		}
		if node == "" {
			continue
		}
		target := dev.Status.State
		switch dev.Status.State {
		case v1alpha1.GPUDeviceStatePendingAssignment:
			if readyNodes[node] {
				target = v1alpha1.GPUDeviceStateAssigned
			}
		case v1alpha1.GPUDeviceStateAssigned:
			if !readyNodes[node] {
				target = v1alpha1.GPUDeviceStatePendingAssignment
			}
		}
		if target == dev.Status.State {
			continue
		}
		orig := dev.DeepCopy()
		dev.Status.State = target
		if err := h.client.Status().Patch(ctx, dev, client.MergeFrom(orig)); err != nil {
			return contracts.Result{}, err
		}
		changed = true
		h.log.V(1).Info("updated device state based on DP validator", "device", dev.Name, "state", target)
	}

	if changed {
		return contracts.Result{}, nil
	}
	return contracts.Result{}, nil
}

func isPodReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
