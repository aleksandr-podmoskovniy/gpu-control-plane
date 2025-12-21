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

package watchers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

type ModuleConfigMapFunc func(context.Context, client.Client, *unstructured.Unstructured) ([]reconcile.Request, error)

type ModuleConfigWatcher struct {
	log      logr.Logger
	store    *moduleconfig.ModuleConfigStore
	mapFunc  ModuleConfigMapFunc
	errorMsg string
	cl       client.Client
}

func NewModuleConfigWatcher(log logr.Logger, store *moduleconfig.ModuleConfigStore, errorMsg string, mapFunc ModuleConfigMapFunc) *ModuleConfigWatcher {
	return &ModuleConfigWatcher{
		log:      log,
		store:    store,
		errorMsg: errorMsg,
		mapFunc:  mapFunc,
	}
}

func (w *ModuleConfigWatcher) enqueue(ctx context.Context, moduleConfig *unstructured.Unstructured) []reconcile.Request {
	if w.store != nil && !w.store.Current().Enabled {
		return nil
	}

	cl := w.cl
	if cl == nil || w.mapFunc == nil {
		return nil
	}

	reqs, err := w.mapFunc(ctx, cl, moduleConfig)
	if err != nil {
		if w.log.GetSink() != nil {
			msg := w.errorMsg
			if msg == "" {
				msg = "module config watcher failed"
			}
			w.log.Error(err, msg)
		}
		return nil
	}

	return reqs
}

func (w *ModuleConfigWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	if w.mapFunc == nil {
		return fmt.Errorf("module config mapper is required")
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(moduleconfig.ModuleConfigGVK)

	w.cl = mgr.GetClient()

	return ctr.Watch(
		source.Kind(cache, obj, handler.TypedEnqueueRequestsFromMapFunc(w.enqueue)),
	)
}
