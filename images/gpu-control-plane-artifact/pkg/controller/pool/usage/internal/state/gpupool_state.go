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

package state

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

type GPUPoolState interface {
	Client() client.Client
	Pool() *v1alpha1.GPUPool
	Used() int32
	UsedSet() bool
	SetUsed(value int32)
}

type GPUPool struct {
	client  client.Client
	pool    *v1alpha1.GPUPool
	used    int32
	usedSet bool
}

func NewGPUPool(client client.Client, pool *v1alpha1.GPUPool) *GPUPool {
	return &GPUPool{client: client, pool: pool}
}

func (s *GPUPool) Client() client.Client {
	return s.client
}

func (s *GPUPool) Pool() *v1alpha1.GPUPool {
	return s.pool
}

func (s *GPUPool) Used() int32 {
	return s.used
}

func (s *GPUPool) UsedSet() bool {
	return s.usedSet
}

func (s *GPUPool) SetUsed(value int32) {
	s.used = value
	s.usedSet = true
}
