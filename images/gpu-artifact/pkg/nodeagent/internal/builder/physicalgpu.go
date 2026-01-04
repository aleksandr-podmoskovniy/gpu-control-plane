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

package builder

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/resource_builder"
)

// PhysicalGPU helps constructing PhysicalGPU objects with common metadata.
type PhysicalGPU struct {
	resource_builder.ResourceBuilder[*gpuv1alpha1.PhysicalGPU]
}

// NewPhysicalGPU creates a new PhysicalGPU builder.
func NewPhysicalGPU(name string) *PhysicalGPU {
	return &PhysicalGPU{
		ResourceBuilder: resource_builder.NewResourceBuilder(&gpuv1alpha1.PhysicalGPU{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}, resource_builder.ResourceBuilderOptions{}),
	}
}

// SetLabels updates the object labels.
func (b *PhysicalGPU) SetLabels(labels map[string]string) {
	b.Resource.SetLabels(labels)
}
