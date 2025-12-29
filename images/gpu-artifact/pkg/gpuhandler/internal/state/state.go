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

package state

import gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"

const (
	// LabelNode marks PhysicalGPU objects with the node name.
	LabelNode = "gpu.deckhouse.io/node"
)

// State provides access to a gpu-handler sync snapshot.
type State interface {
	NodeName() string
	All() []gpuv1alpha1.PhysicalGPU
	SetAll(devices []gpuv1alpha1.PhysicalGPU)
	Ready() []gpuv1alpha1.PhysicalGPU
	SetReady(devices []gpuv1alpha1.PhysicalGPU)
}

type state struct {
	nodeName string
	all      []gpuv1alpha1.PhysicalGPU
	ready    []gpuv1alpha1.PhysicalGPU
}

// New initializes the state for a single sync loop.
func New(nodeName string) State {
	return &state{nodeName: nodeName}
}

func (s *state) NodeName() string {
	return s.nodeName
}

func (s *state) All() []gpuv1alpha1.PhysicalGPU {
	return s.all
}

func (s *state) SetAll(devices []gpuv1alpha1.PhysicalGPU) {
	s.all = devices
}

func (s *state) Ready() []gpuv1alpha1.PhysicalGPU {
	return s.ready
}

func (s *state) SetReady(devices []gpuv1alpha1.PhysicalGPU) {
	s.ready = devices
}
