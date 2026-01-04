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

	resourcev1 "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	controllerwatchers "github.com/aleksandr-podmoskovniy/gpu/pkg/controller/watchers"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

// ResourceSliceWatcher wires ResourceSlice changes to ResourceClaim reconciles.
type ResourceSliceWatcher struct {
	reader client.Reader
}

// NewResourceSliceWatcher creates a new ResourceSliceWatcher.
func NewResourceSliceWatcher(reader client.Reader) *ResourceSliceWatcher {
	return &ResourceSliceWatcher{reader: reader}
}

// Watch registers controller watches.
func (w *ResourceSliceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	watcher := controllerwatchers.NewObjectRefWatcher(resourceSliceUpdateFilter{}, resourceSliceEnqueuer{reader: w.reader})
	return watcher.Run(mgr, ctr)
}

type resourceSliceUpdateFilter struct{}

func (resourceSliceUpdateFilter) FilterUpdateEvents(_ event.UpdateEvent) bool {
	return true
}

type resourceSliceEnqueuer struct {
	reader client.Reader
}

func (e resourceSliceEnqueuer) GetEnqueueFrom() client.Object {
	return &resourcev1.ResourceSlice{}
}

func (e resourceSliceEnqueuer) EnqueueRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	slice, ok := obj.(*resourcev1.ResourceSlice)
	if !ok || slice.Spec.Driver != allocator.DefaultDriverName {
		return nil
	}

	list := &resourcev1.ResourceClaimList{}
	if err := e.reader.List(ctx, list); err != nil {
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return reqs
}
