/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

import (
	"context"

	resourceapi "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
)

const (
	deviceStatusConditionReady  = "Ready"
	deviceStatusReasonPrepared  = "Prepared"
	deviceStatusMessagePrepared = "device prepared"
)

func (d *Driver) updateClaimDeviceStatus(ctx context.Context, claim *resourceapi.ResourceClaim) error {
	if d == nil || d.kubeClient == nil || claim == nil {
		return nil
	}
	statuses := buildDeviceStatuses(claim, d.driverName)
	if len(statuses) == 0 {
		return nil
	}
	return d.patchClaimDeviceStatuses(ctx, claim.Namespace, claim.Name, statuses)
}

func (d *Driver) clearClaimDeviceStatus(ctx context.Context, claim kubeletplugin.NamespacedObject) error {
	if d == nil || d.kubeClient == nil {
		return nil
	}
	return d.patchClaimDeviceStatuses(ctx, claim.Namespace, claim.Name, nil)
}

func (d *Driver) patchClaimDeviceStatuses(ctx context.Context, namespace, name string, statuses []resourceapi.AllocatedDeviceStatus) error {
	if namespace == "" || name == "" {
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := d.kubeClient.ResourceV1().ResourceClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}

		current.Status.Devices = mergeDeviceStatuses(current.Status.Devices, statuses, d.driverName)
		_, err = d.kubeClient.ResourceV1().ResourceClaims(namespace).UpdateStatus(ctx, current, metav1.UpdateOptions{})
		return err
	})
}
