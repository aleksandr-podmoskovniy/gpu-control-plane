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
)

// DeviceClassWatcher wires DeviceClass changes to ResourceClaim reconciles.
type DeviceClassWatcher struct {
	reader client.Reader
}

// NewDeviceClassWatcher creates a new DeviceClassWatcher.
func NewDeviceClassWatcher(reader client.Reader) *DeviceClassWatcher {
	return &DeviceClassWatcher{reader: reader}
}

// Watch registers controller watches.
func (w *DeviceClassWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	watcher := controllerwatchers.NewObjectRefWatcher(deviceClassUpdateFilter{}, deviceClassEnqueuer{reader: w.reader})
	return watcher.Run(mgr, ctr)
}

type deviceClassUpdateFilter struct{}

func (deviceClassUpdateFilter) FilterUpdateEvents(_ event.UpdateEvent) bool {
	return true
}

type deviceClassEnqueuer struct {
	reader client.Reader
}

func (e deviceClassEnqueuer) GetEnqueueFrom() client.Object {
	return &resourcev1.DeviceClass{}
}

func (e deviceClassEnqueuer) EnqueueRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	deviceClass, ok := obj.(*resourcev1.DeviceClass)
	if !ok || deviceClass.Name == "" {
		return nil
	}

	list := &resourcev1.ResourceClaimList{}
	if err := e.reader.List(ctx, list); err != nil {
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		claim := &list.Items[i]
		if claimUsesDeviceClass(claim, deviceClass.Name) {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(claim)})
		}
	}
	return reqs
}

func claimUsesDeviceClass(claim *resourcev1.ResourceClaim, className string) bool {
	for _, req := range claim.Spec.Devices.Requests {
		if req.Exactly != nil && req.Exactly.DeviceClassName == className {
			return true
		}
		for _, sub := range req.FirstAvailable {
			if sub.DeviceClassName == className {
				return true
			}
		}
	}
	return false
}
