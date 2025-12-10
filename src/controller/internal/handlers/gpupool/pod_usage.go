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
	"sort"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
)

// PodUsageHandler updates GPUDevice states based on scheduled pods requesting pool resources.
// We approximate per-device assignment by deterministically picking devices on the target node.
// States considered: Assigned -> Reserved while pod scheduled, InUse when pod Ready/Running.
type PodUsageHandler struct {
	log    logr.Logger
	client client.Client
}

func NewPodUsageHandler(log logr.Logger, cl client.Client) *PodUsageHandler {
	return &PodUsageHandler{log: log, client: cl}
}

func (h *PodUsageHandler) Name() string { return "pod-usage" }

func (h *PodUsageHandler) HandlePool(ctx context.Context, pool *v1alpha1.GPUPool) (contracts.Result, error) {
	if h.client == nil {
		return contracts.Result{}, nil
	}

	resourcePrefix := "gpu.deckhouse.io/"
	if pool.Namespace == "" || strings.EqualFold(pool.Kind, "ClusterGPUPool") {
		resourcePrefix = "cluster.gpu.deckhouse.io/"
	}
	resourceName := corev1.ResourceName(resourcePrefix + pool.Name)

	// Collect pods with pool resource requests.
	pods := &corev1.PodList{}
	if err := h.client.List(ctx, pods); err != nil {
		return contracts.Result{}, err
	}

	// requested[node] = {reserved, inUse}
	type counts struct {
		reserved int32
		inUse    int32
	}
	requested := map[string]counts{}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Spec.NodeName == "" {
			continue
		}
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			continue
		}
		qty := totalResource(pod, resourceName)
		if qty == 0 {
			continue
		}
		c := requested[pod.Spec.NodeName]
		if pod.Status.Phase == corev1.PodRunning && podReady(pod) {
			c.inUse += qty
		} else {
			c.reserved += qty
		}
		requested[pod.Spec.NodeName] = c
	}

	// Collect devices assigned to this pool.
	devices := &v1alpha1.GPUDeviceList{}
	if err := h.client.List(ctx, devices); err != nil {
		return contracts.Result{}, err
	}
	byNode := map[string][]*v1alpha1.GPUDevice{}
	for i := range devices.Items {
		dev := &devices.Items[i]
		if dev.Annotations[assignmentAnnotation] != pool.Name {
			continue
		}
		if dev.Status.State != v1alpha1.GPUDeviceStateAssigned &&
			dev.Status.State != v1alpha1.GPUDeviceStateReserved &&
			dev.Status.State != v1alpha1.GPUDeviceStateInUse {
			continue
		}
		node := dev.Status.NodeName
		if node == "" {
			node = dev.Labels["kubernetes.io/hostname"]
		}
		if node == "" {
			continue
		}
		byNode[node] = append(byNode[node], dev)
	}

	var changed bool
	for node, devs := range byNode {
		sort.Slice(devs, func(i, j int) bool { return devs[i].Status.InventoryID < devs[j].Status.InventoryID })
		cnt := requested[node]
		for _, dev := range devs {
			target := v1alpha1.GPUDeviceStateAssigned
			switch {
			case cnt.inUse > 0:
				target = v1alpha1.GPUDeviceStateInUse
				cnt.inUse--
			case cnt.reserved > 0:
				target = v1alpha1.GPUDeviceStateReserved
				cnt.reserved--
			}
			if dev.Status.State == target {
				continue
			}
			orig := dev.DeepCopy()
			dev.Status.State = target
			if err := h.client.Status().Patch(ctx, dev, client.MergeFrom(orig)); err != nil {
				return contracts.Result{}, err
			}
			changed = true
		}
	}

	if changed {
		return contracts.Result{}, nil
	}
	return contracts.Result{}, nil
}

func totalResource(pod *corev1.Pod, res corev1.ResourceName) int32 {
	var total int64
	acc := func(c corev1.Container) {
		if q, ok := c.Resources.Limits[res]; ok {
			total += q.Value()
			return
		}
		if q, ok := c.Resources.Requests[res]; ok {
			total += q.Value()
		}
	}
	for _, c := range pod.Spec.InitContainers {
		acc(c)
	}
	for _, c := range pod.Spec.Containers {
		acc(c)
	}
	return int32(total)
}

func podReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
