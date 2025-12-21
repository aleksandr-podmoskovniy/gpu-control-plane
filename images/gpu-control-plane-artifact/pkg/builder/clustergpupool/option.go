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

package clustergpupool

import (
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/builder/meta"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

type Option func(pool *v1alpha1.ClusterGPUPool)

var (
	WithName         = meta.WithName[*v1alpha1.ClusterGPUPool]
	WithGenerateName = meta.WithGenerateName[*v1alpha1.ClusterGPUPool]
	WithLabel        = meta.WithLabel[*v1alpha1.ClusterGPUPool]
	WithLabels       = meta.WithLabels[*v1alpha1.ClusterGPUPool]
	WithAnnotation   = meta.WithAnnotation[*v1alpha1.ClusterGPUPool]
	WithAnnotations  = meta.WithAnnotations[*v1alpha1.ClusterGPUPool]
)

func WithSpec(spec v1alpha1.GPUPoolSpec) Option {
	return func(pool *v1alpha1.ClusterGPUPool) {
		pool.Spec = spec
	}
}

