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

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/logger"
)

func SetupController(
	ctx context.Context,
	mgr ctrl.Manager,
	log logr.Logger,
	store *ModuleConfigStore,
) error {
	baseLog := log.WithName("moduleconfig")
	r, err := New(mgr.GetClient(), baseLog, store)
	if err != nil {
		return err
	}

	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: 1,
		RecoverPanic:            ptr.To(true),
		LogConstructor:          logger.NewConstructor(baseLog),
	})
	if err != nil {
		return err
	}

	if err := r.SetupController(ctx, mgr, c); err != nil {
		return err
	}

	baseLog.Info("Initialized ModuleConfig controller")
	return nil
}
