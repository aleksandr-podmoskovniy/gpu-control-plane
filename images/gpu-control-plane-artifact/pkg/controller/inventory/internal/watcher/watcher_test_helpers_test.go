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

package watcher

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type fakeCache struct{ cache.Cache }

type stubManager struct {
	manager.Manager
	cache cache.Cache
}

func (m *stubManager) GetCache() cache.Cache { return m.cache }

type stubController struct {
	watched []source.Source
	err     error
}

func (c *stubController) Reconcile(context.Context, reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (c *stubController) Watch(src source.Source) error {
	if c.err != nil {
		return c.err
	}
	c.watched = append(c.watched, src)
	return nil
}

func (c *stubController) Start(context.Context) error { return nil }

func (c *stubController) GetLogger() logr.Logger { return logr.Discard() }

var _ controller.Controller = (*stubController)(nil)

