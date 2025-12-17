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

package inventory

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/pool"
)

const (
	namespacedAssignmentAnnotation = "gpu.deckhouse.io/assignment"
	clusterAssignmentAnnotation    = "cluster.gpu.deckhouse.io/assignment"
)

type GPUDeviceAssignmentValidator struct {
	log    logr.Logger
	client client.Client
}

func NewGPUDeviceAssignmentValidator(log logr.Logger, c client.Client) *GPUDeviceAssignmentValidator {
	return &GPUDeviceAssignmentValidator{
		log:    log.WithName("gpudevice-webhook"),
		client: c,
	}
}

func (v *GPUDeviceAssignmentValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (cradmission.Warnings, error) {
	device, ok := obj.(*v1alpha1.GPUDevice)
	if !ok {
		return nil, fmt.Errorf("expected a GPUDevice but got a %T", obj)
	}

	namespacedPool := strings.TrimSpace(device.Annotations[namespacedAssignmentAnnotation])
	clusterPool := strings.TrimSpace(device.Annotations[clusterAssignmentAnnotation])
	if namespacedPool != "" && clusterPool != "" {
		return nil, fmt.Errorf("only one assignment annotation is allowed (namespaced or cluster)")
	}
	if namespacedPool == "" && clusterPool == "" {
		return nil, nil
	}

	return nil, v.validateAssignment(ctx, device, namespacedPool, clusterPool)
}

func (v *GPUDeviceAssignmentValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (cradmission.Warnings, error) {
	oldDevice, ok := oldObj.(*v1alpha1.GPUDevice)
	if !ok {
		return nil, fmt.Errorf("expected an old GPUDevice but got a %T", oldObj)
	}
	newDevice, ok := newObj.(*v1alpha1.GPUDevice)
	if !ok {
		return nil, fmt.Errorf("expected a new GPUDevice but got a %T", newObj)
	}

	oldNamespacedPool := strings.TrimSpace(oldDevice.Annotations[namespacedAssignmentAnnotation])
	oldClusterPool := strings.TrimSpace(oldDevice.Annotations[clusterAssignmentAnnotation])
	newNamespacedPool := strings.TrimSpace(newDevice.Annotations[namespacedAssignmentAnnotation])
	newClusterPool := strings.TrimSpace(newDevice.Annotations[clusterAssignmentAnnotation])

	if oldNamespacedPool == newNamespacedPool && oldClusterPool == newClusterPool {
		return nil, nil
	}

	if newNamespacedPool != "" && newClusterPool != "" {
		return nil, fmt.Errorf("only one assignment annotation is allowed (namespaced or cluster)")
	}
	if newNamespacedPool == "" && newClusterPool == "" {
		return nil, nil
	}

	return nil, v.validateAssignment(ctx, newDevice, newNamespacedPool, newClusterPool)
}

func (v *GPUDeviceAssignmentValidator) ValidateDelete(_ context.Context, _ runtime.Object) (cradmission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

func (v *GPUDeviceAssignmentValidator) validateAssignment(ctx context.Context, device *v1alpha1.GPUDevice, namespacedPool, clusterPool string) error {
	if v.client == nil {
		return fmt.Errorf("webhook client is not configured")
	}

	if strings.EqualFold(device.Labels["gpu.deckhouse.io/ignore"], "true") {
		return fmt.Errorf("device is marked as ignored")
	}

	if device.Status.State != v1alpha1.GPUDeviceStateReady {
		return fmt.Errorf("device state must be Ready, got %s", device.Status.State)
	}

	if strings.TrimSpace(device.Status.Hardware.UUID) == "" || strings.TrimSpace(device.Status.Hardware.PCI.Address) == "" {
		return fmt.Errorf("device inventory is incomplete (uuid/pci address), wait for inventory sync")
	}

	if clusterPool != "" {
		poolObj := &v1alpha1.ClusterGPUPool{}
		if err := v.client.Get(ctx, client.ObjectKey{Name: clusterPool}, poolObj); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("assigned ClusterGPUPool %q not found", clusterPool)
			}
			return err
		}
		if matchesDeviceSelector(device, poolObj.Spec.DeviceSelector) {
			return nil
		}
		return fmt.Errorf("device does not match selector of ClusterGPUPool %q", poolObj.Name)
	}

	poolObj, err := resolveNamespacedPoolByName(ctx, v.client, namespacedPool)
	if err != nil {
		return err
	}

	if matchesDeviceSelector(device, poolObj.Spec.DeviceSelector) {
		return nil
	}
	return fmt.Errorf("device does not match selector of pool %q", poolObj.Name)
}

func matchesDeviceSelector(dev *v1alpha1.GPUDevice, sel *v1alpha1.GPUPoolDeviceSelector) bool {
	if dev == nil {
		return false
	}
	candidates := pool.FilterDevices([]v1alpha1.GPUDevice{*dev}, sel)
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

var _ cradmission.CustomValidator = (*GPUDeviceAssignmentValidator)(nil)

