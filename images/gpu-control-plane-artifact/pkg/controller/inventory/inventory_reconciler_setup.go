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

package inventory

import (
	"context"
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/indexer"
	invservice "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/service"
	invwatcher "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/watcher"
)

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

func (r *Reconciler) SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	r.client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	r.recorder = mgr.GetEventRecorderFor(ControllerName)
	if r.detectionCollector == nil {
		r.detectionCollector = invservice.NewDetectionCollector(r.client)
	}
	if r.cleanupService == nil {
		r.cleanupService = invservice.NewCleanupService(r.client, r.recorder)
	}
	if r.deviceService == nil {
		r.deviceService = invservice.NewDeviceService(r.client, r.scheme, r.recorder, r.deviceHandlers)
	}
	if r.inventoryService == nil {
		r.inventoryService = invservice.NewInventoryService(r.client, r.scheme, r.recorder)
	}

	if idx := mgr.GetFieldIndexer(); idx != nil {
		obj, field, extract := indexer.IndexGPUDeviceByNode()
		if err := idx.IndexField(ctx, obj, field, extract); err != nil {
			return err
		}
	}

	for _, w := range []Watcher{
		invwatcher.NewNodeWatcher(),
		invwatcher.NewNodeFeatureWatcher(),
		invwatcher.NewGFDPodWatcher(),
		invwatcher.NewNodeStateWatcher(),
	} {
		if err := w.Watch(mgr, ctr); err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}
