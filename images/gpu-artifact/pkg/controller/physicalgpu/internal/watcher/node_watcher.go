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

package watcher

import (
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// NodeWatcher is a placeholder watcher for future Node watches.
type NodeWatcher struct{}

// NewNodeWatcher creates a new NodeWatcher.
func NewNodeWatcher() *NodeWatcher {
	return &NodeWatcher{}
}

// Watch registers controller watches.
func (w *NodeWatcher) Watch(_ manager.Manager, _ controller.Controller) error {
	return nil
}
