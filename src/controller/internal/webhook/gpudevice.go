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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/handlers/gpupool"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/indexer"
)

const (
	namespacedAssignmentAnnotation = "gpu.deckhouse.io/assignment"
	clusterAssignmentAnnotation    = "cluster.gpu.deckhouse.io/assignment"
)

type gpuDeviceAssignmentValidator struct {
	log     logr.Logger
	decoder cradmission.Decoder
	client  client.Client
}

func newGPUDeviceAssignmentValidator(log logr.Logger, decoder cradmission.Decoder, c client.Client) cradmission.Handler {
	return &gpuDeviceAssignmentValidator{
		log:     log.WithName("gpu-device-assignment"),
		decoder: decoder,
		client:  c,
	}
}

func (h *gpuDeviceAssignmentValidator) Handle(ctx context.Context, req cradmission.Request) cradmission.Response {
	if req.Operation != admv1.Create && req.Operation != admv1.Update {
		return cradmission.Allowed("")
	}

	device := &v1alpha1.GPUDevice{}
	if err := h.decoder.Decode(req, device); err != nil {
		return cradmission.Errored(http.StatusUnprocessableEntity, err)
	}

	namespacedPool := strings.TrimSpace(device.Annotations[namespacedAssignmentAnnotation])
	clusterPool := strings.TrimSpace(device.Annotations[clusterAssignmentAnnotation])

	if namespacedPool != "" && clusterPool != "" {
		return cradmission.Denied("only one assignment annotation is allowed (namespaced or cluster)")
	}

	assignmentSet := namespacedPool != "" || clusterPool != ""
	assignmentChanged := false
	switch req.Operation {
	case admv1.Create:
		assignmentChanged = assignmentSet
		case admv1.Update:
			oldDevice := &v1alpha1.GPUDevice{}
			if len(req.OldObject.Raw) > 0 {
				if err := h.decoder.DecodeRaw(req.OldObject, oldDevice); err != nil {
					return cradmission.Errored(http.StatusUnprocessableEntity, err)
				}
			} else if h.client != nil {
				name := strings.TrimSpace(req.Name)
				if name == "" {
					name = strings.TrimSpace(device.Name)
				}
				if name == "" {
					assignmentChanged = false
					break
				}
				// Some update paths might omit oldObject; fall back to the stored object to diff assignment.
				if err := h.client.Get(ctx, client.ObjectKey{Name: name}, oldDevice); err != nil {
					if apierrors.IsNotFound(err) {
						assignmentChanged = false
						break
					}
				return cradmission.Errored(http.StatusInternalServerError, err)
			}
		} else {
			assignmentChanged = false
			break
		}

		oldNamespacedPool := strings.TrimSpace(oldDevice.Annotations[namespacedAssignmentAnnotation])
		oldClusterPool := strings.TrimSpace(oldDevice.Annotations[clusterAssignmentAnnotation])
		assignmentChanged = oldNamespacedPool != namespacedPool || oldClusterPool != clusterPool
	}

	if !assignmentChanged {
		return cradmission.Allowed("")
	}

	if namespacedPool == "" && clusterPool == "" {
		return cradmission.Allowed("")
	}

	if strings.EqualFold(device.Labels["gpu.deckhouse.io/ignore"], "true") {
		return cradmission.Denied("device is marked as ignored")
	}

	if device.Status.State != v1alpha1.GPUDeviceStateReady {
		return cradmission.Denied(fmt.Sprintf("device state must be Ready, got %s", device.Status.State))
	}

	if strings.TrimSpace(device.Status.Hardware.UUID) == "" || strings.TrimSpace(device.Status.Hardware.PCI.Address) == "" {
		return cradmission.Denied("device inventory is incomplete (uuid/pci address), wait for inventory sync")
	}

	if clusterPool != "" {
		pool := &v1alpha1.ClusterGPUPool{}
		if err := h.client.Get(ctx, client.ObjectKey{Name: clusterPool}, pool); err != nil {
			if apierrors.IsNotFound(err) {
				return cradmission.Denied(fmt.Sprintf("assigned ClusterGPUPool %q not found", clusterPool))
			}
			return cradmission.Errored(http.StatusInternalServerError, err)
		}
		if matchesDeviceSelector(device, pool.Spec.DeviceSelector) {
			return cradmission.Allowed("")
		}
		return cradmission.Denied(fmt.Sprintf("device does not match selector of ClusterGPUPool %q", pool.Name))
	}

	pool, err := resolveNamespacedPoolByName(ctx, h.client, namespacedPool)
	if err != nil {
		return cradmission.Denied(err.Error())
	}

	if matchesDeviceSelector(device, pool.Spec.DeviceSelector) {
		return cradmission.Allowed("")
	}

	return cradmission.Denied(fmt.Sprintf("device does not match selector of pool %q", pool.Name))
}

func matchesDeviceSelector(dev *v1alpha1.GPUDevice, sel *v1alpha1.GPUPoolDeviceSelector) bool {
	if dev == nil {
		return false
	}
	candidates := gpupool.FilterDevices([]v1alpha1.GPUDevice{*dev}, sel)
	return len(candidates) == 1
}

func resolveNamespacedPoolByName(ctx context.Context, c client.Client, name string) (*v1alpha1.GPUPool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("assigned pool name is empty")
	}
	if c == nil {
		return nil, fmt.Errorf("GPUPool %q: webhook client is not configured", name)
	}

	list := &v1alpha1.GPUPoolList{}
	if err := c.List(ctx, list, client.MatchingFields{indexer.GPUPoolNameField: name}); err != nil {
		// Fake client requires explicit indexes; fall back to full scan in tests and defensive scenarios.
		if !isMissingIndexError(err) {
			return nil, fmt.Errorf("list GPUPools by name: %w", err)
		}
		if err := c.List(ctx, list); err != nil {
			return nil, fmt.Errorf("list GPUPools: %w", err)
		}
	}

	var matches []v1alpha1.GPUPool
	for _, pool := range list.Items {
		if pool.Name == name {
			matches = append(matches, pool)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("assigned pool %q not found", name)
	}
	if len(matches) > 1 {
		namespaces := make([]string, 0, len(matches))
		for _, pool := range matches {
			if pool.Namespace != "" {
				namespaces = append(namespaces, pool.Namespace)
			}
		}
		sort.Strings(namespaces)
		return nil, fmt.Errorf("assigned pool %q is ambiguous (found in namespaces: %s)", name, strings.Join(namespaces, ", "))
	}
	return &matches[0], nil
}

func isMissingIndexError(err error) bool {
	if err == nil {
		return false
	}
	// controller-runtime fake client returns a plain error string for missing indexes.
	msg := err.Error()
	return strings.Contains(msg, "no index with name") && strings.Contains(msg, "has been registered")
}
