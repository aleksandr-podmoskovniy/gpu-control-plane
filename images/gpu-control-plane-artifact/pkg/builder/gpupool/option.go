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
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/builder/meta"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

type Option func(pool *v1alpha1.GPUPool)

var (
	WithName         = meta.WithName[*v1alpha1.GPUPool]
	WithGenerateName = meta.WithGenerateName[*v1alpha1.GPUPool]
	WithNamespace    = meta.WithNamespace[*v1alpha1.GPUPool]
	WithLabel        = meta.WithLabel[*v1alpha1.GPUPool]
	WithLabels       = meta.WithLabels[*v1alpha1.GPUPool]
	WithAnnotation   = meta.WithAnnotation[*v1alpha1.GPUPool]
	WithAnnotations  = meta.WithAnnotations[*v1alpha1.GPUPool]
)

func WithSpec(spec v1alpha1.GPUPoolSpec) Option {
	return func(pool *v1alpha1.GPUPool) {
		pool.Spec = spec
	}
}

