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
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	physicalgpuindexer "github.com/aleksandr-podmoskovniy/gpu/pkg/controller/physicalgpu/internal/indexer"
	controllerwatchers "github.com/aleksandr-podmoskovniy/gpu/pkg/controller/watchers"
)

// NodeWatcher wires Node changes to PhysicalGPU reconcile requests.
type NodeWatcher struct {
	reader client.Reader
}

// NewNodeWatcher creates a new NodeWatcher.
func NewNodeWatcher(reader client.Reader) *NodeWatcher {
	return &NodeWatcher{reader: reader}
}

// Watch registers controller watches.
func (w *NodeWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	watcher := controllerwatchers.NewObjectRefWatcher(nodeUpdateFilter{}, nodeEnqueuer{reader: w.reader})
	return watcher.Run(mgr, ctr)
}

type nodeUpdateFilter struct{}

func (nodeUpdateFilter) FilterUpdateEvents(_ event.UpdateEvent) bool {
	return true
}

type nodeEnqueuer struct {
	reader client.Reader
}

func (e nodeEnqueuer) GetEnqueueFrom() client.Object {
	return &corev1.Node{}
}

func (e nodeEnqueuer) EnqueueRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	node, ok := obj.(*corev1.Node)
	if !ok || node.Name == "" {
		return nil
	}

	list := &gpuv1alpha1.PhysicalGPUList{}
	if err := e.reader.List(ctx, list, client.MatchingFields{physicalgpuindexer.FieldNodeName: node.Name}); err != nil {
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKey{Name: list.Items[i].Name}})
	}
	return reqs
}
