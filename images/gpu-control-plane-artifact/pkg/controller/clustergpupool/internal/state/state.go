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

type PoolState interface {
	Client() client.Client
	Pool() *v1alpha1.GPUPool
}

type State struct {
	client client.Client
	pool   *v1alpha1.GPUPool
}

func New(client client.Client, pool *v1alpha1.GPUPool) *State {
	return &State{client: client, pool: pool}
}

func (s *State) Client() client.Client {
	return s.client
}

func (s *State) Pool() *v1alpha1.GPUPool {
	return s.pool
}

