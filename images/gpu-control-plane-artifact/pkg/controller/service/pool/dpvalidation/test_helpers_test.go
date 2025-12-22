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

package dpvalidation

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonannotations "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/annotations"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
)

func withPoolDeviceIndexes(builder *fake.ClientBuilder) *fake.ClientBuilder {
	if builder == nil {
		return nil
	}

	builder = builder.WithIndex(&v1alpha1.GPUDevice{}, indexer.GPUDevicePoolRefNameField, func(obj client.Object) []string {
		dev, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || dev.Status.PoolRef == nil || dev.Status.PoolRef.Name == "" {
			return nil
		}
		return []string{dev.Status.PoolRef.Name}
	})

	builder = builder.WithIndex(&v1alpha1.GPUDevice{}, indexer.GPUDeviceNamespacedAssignmentField, func(obj client.Object) []string {
		dev, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || dev.Annotations == nil {
			return nil
		}
		if value := dev.Annotations[commonannotations.GPUDeviceAssignment]; value != "" {
			return []string{value}
		}
		return nil
	})

	builder = builder.WithIndex(&v1alpha1.GPUDevice{}, indexer.GPUDeviceClusterAssignmentField, func(obj client.Object) []string {
		dev, ok := obj.(*v1alpha1.GPUDevice)
		if !ok || dev.Annotations == nil {
			return nil
		}
		if value := dev.Annotations[commonannotations.ClusterGPUDeviceAssignment]; value != "" {
			return []string{value}
		}
		return nil
	})

	return builder
}
