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

type NodeState interface {
	Client() client.Client
	Inventory() *v1alpha1.GPUNodeState
}

type State struct {
	client    client.Client
	inventory *v1alpha1.GPUNodeState
}

func New(client client.Client, inventory *v1alpha1.GPUNodeState) *State {
	return &State{client: client, inventory: inventory}
}

func (s *State) Client() client.Client {
	return s.client
}

func (s *State) Inventory() *v1alpha1.GPUNodeState {
	return s.inventory
}

