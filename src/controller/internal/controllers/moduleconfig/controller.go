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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/internal/config"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controllerbuilder"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
	moduleconfigpkg "github.com/aleksandr-podmoskovniy/gpu-control-plane/pkg/moduleconfig"
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
	store  *config.ModuleConfigStore
	build  func(ctrl.Manager) controllerbuilder.Builder
}

func New(log logr.Logger, store *config.ModuleConfigStore) (*Reconciler, error) {
	if store == nil {
		return nil, fmt.Errorf("module config store must be provided")
	}
	rec := &Reconciler{log: log, store: store}
	rec.build = controllerbuilder.NewManagedBy
	return rec, nil
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	r.client = mgr.GetClient()

	base := &unstructured.Unstructured{}
	base.SetGroupVersionKind(ModuleConfigGVK)

	options := controller.Options{
		MaxConcurrentReconciles: 1,
		RecoverPanic:            ptr.To(true),
		LogConstructor:          logger.NewConstructor(r.log),
	}

	ctrlBuilder := r.build(mgr).
		Named(controllerName).
		For(base).
		WithOptions(options)

	return ctrlBuilder.Complete(r)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := crlog.FromContext(ctx).WithValues("moduleConfig", req.Name)
	ctx = logr.NewContext(ctx, log)

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(ModuleConfigGVK)

	if err := r.client.Get(ctx, types.NamespacedName{Name: moduleConfigName}, obj); err != nil {
		if apierrors.IsNotFound(err) {
			r.store.Update(moduleconfigpkg.DefaultState())
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	state, err := r.extractState(obj)
	if err != nil {
		log.Error(err, "parse module configuration")
		return ctrl.Result{}, err
	}

	r.store.Update(state)
	return ctrl.Result{}, nil
}

func (r *Reconciler) extractState(obj *unstructured.Unstructured) (moduleconfigpkg.State, error) {
	var enabledPtr *bool
	if enabled, found, err := unstructured.NestedBool(obj.Object, "spec", "enabled"); err == nil && found {
		copy := enabled
		enabledPtr = &copy
	}

	settings, _, err := unstructured.NestedMap(obj.Object, "spec", "settings")
	if err != nil {
		return moduleconfigpkg.State{}, fmt.Errorf("read spec.settings: %w", err)
	}

	input := moduleconfigpkg.Input{
		Enabled:  enabledPtr,
		Settings: settings,
	}
	return moduleconfigpkg.Parse(input)
}
