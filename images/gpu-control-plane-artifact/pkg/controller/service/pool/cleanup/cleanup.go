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

package cleanup

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
)

// PoolResources removes per-pool workloads when backend/provider changes.
func PoolResources(ctx context.Context, c client.Client, namespace, poolName string) error {
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("nvidia-device-plugin-%s", poolName),
		Namespace: namespace,
	}}
	if err := commonobject.DeleteObject(ctx, c, ds); err != nil {
		return err
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("nvidia-device-plugin-%s-config", poolName),
		Namespace: namespace,
	}}
	if err := commonobject.DeleteObject(ctx, c, cm); err != nil {
		return err
	}
	if err := MIGResources(ctx, c, namespace, poolName); err != nil {
		return err
	}
	validator := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("nvidia-operator-validator-%s", poolName),
		Namespace: namespace,
	}}
	if err := commonobject.DeleteObject(ctx, c, validator); err != nil {
		return err
	}
	return nil
}

// MIGResources removes MIG manager workloads for the pool.
func MIGResources(ctx context.Context, c client.Client, namespace, poolName string) error {
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("nvidia-mig-manager-%s", poolName),
		Namespace: namespace,
	}}
	if err := commonobject.DeleteObject(ctx, c, ds); err != nil {
		return err
	}
	for _, name := range []string{
		fmt.Sprintf("nvidia-mig-manager-%s-config", poolName),
		fmt.Sprintf("nvidia-mig-manager-%s-scripts", poolName),
		fmt.Sprintf("nvidia-mig-manager-%s-gpu-clients", poolName),
	} {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		}}
		if err := commonobject.DeleteObject(ctx, c, cm); err != nil {
			return err
		}
	}
	return nil
}
