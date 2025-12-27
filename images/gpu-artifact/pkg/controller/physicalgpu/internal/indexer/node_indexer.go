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

package indexer

import (
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	controllerindexer "github.com/aleksandr-podmoskovniy/gpu/pkg/controller/indexer"
)

const (
	// FieldNodeName indexes PhysicalGPU objects by status.nodeInfo.nodeName.
	FieldNodeName = "status.nodeInfo.nodeName"
)

var registerOnce sync.Once

// Register wires PhysicalGPU indexers into the global registry.
func Register() {
	registerOnce.Do(func() {
		controllerindexer.IndexGetters = append(controllerindexer.IndexGetters, func() (client.Object, string, client.IndexerFunc) {
			return &gpuv1alpha1.PhysicalGPU{}, FieldNodeName, func(obj client.Object) []string {
				pgpu, ok := obj.(*gpuv1alpha1.PhysicalGPU)
				if !ok || pgpu.Status.NodeInfo == nil || pgpu.Status.NodeInfo.NodeName == "" {
					return nil
				}
				return []string{pgpu.Status.NodeInfo.NodeName}
			}
		})
	})
}
