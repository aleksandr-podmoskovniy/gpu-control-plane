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

package moduleconfig

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
)

const (
	controllerName   = "module-config-controller"
	moduleConfigName = "gpu-control-plane"
)

var ModuleConfigGVK = schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1alpha1", Kind: "ModuleConfig"}

// Reconciler watches ModuleConfig changes and updates shared store.
type Reconciler struct {
	client client.Client
	log    logr.Logger
	store  *ModuleConfigStore
}

func New(client client.Client, log logr.Logger, store *ModuleConfigStore) (*Reconciler, error) {
	if store == nil {
		return nil, fmt.Errorf("module config store must be provided")
	}
	rec := &Reconciler{client: client, log: log, store: store}
	return rec, nil
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	cache := mgr.GetCache()
	if cache == nil {
		return fmt.Errorf("manager cache is required")
	}

	base := &unstructured.Unstructured{}
	base.SetGroupVersionKind(ModuleConfigGVK)
	return ctr.Watch(
		source.Kind(cache, base, &handler.TypedEnqueueRequestForObject[*unstructured.Unstructured]{}),
	)
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := crlog.FromContext(ctx).WithValues("moduleConfig", req.Name)
	ctx = logr.NewContext(ctx, log)

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(ModuleConfigGVK)

	obj, err := commonobject.FetchObject(ctx, types.NamespacedName{Name: moduleConfigName}, r.client, obj)
	if err != nil {
		return reconcile.Result{}, err
	}
	if obj == nil {
		r.store.Update(DefaultState())
		return reconcile.Result{}, nil
	}

	state, err := r.extractState(obj)
	if err != nil {
		log.Error(err, "parse module configuration")
		return reconcile.Result{}, err
	}

	r.store.Update(state)
	return reconcile.Result{}, nil
}

func (r *Reconciler) extractState(obj *unstructured.Unstructured) (State, error) {
	var enabledPtr *bool
	if enabled, found, err := unstructured.NestedBool(obj.Object, "spec", "enabled"); err == nil && found {
		copy := enabled
		enabledPtr = &copy
	}

	settings, _, err := unstructured.NestedMap(obj.Object, "spec", "settings")
	if err != nil {
		return State{}, fmt.Errorf("read spec.settings: %w", err)
	}

	input := Input{
		Enabled:  enabledPtr,
		Settings: settings,
	}
	return Parse(input)
}
