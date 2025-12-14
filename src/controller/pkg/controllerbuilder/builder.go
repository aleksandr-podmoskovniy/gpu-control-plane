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

package controllerbuilder

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Builder is a thin wrapper over controller-runtime builder.Builder that makes it easier
// to inject fake builders into controller tests.
type Builder interface {
	Named(string) Builder
	For(client.Object, ...builder.ForOption) Builder
	Owns(client.Object, ...builder.OwnsOption) Builder
	WatchesRawSource(source.Source) Builder
	WithOptions(controller.Options) Builder
	Complete(reconcile.Reconciler) error
}

type runtimeBuilder struct {
	delegate *builder.Builder
}

func NewManagedBy(mgr ctrl.Manager) Builder {
	return &runtimeBuilder{delegate: ctrl.NewControllerManagedBy(mgr)}
}

func (b *runtimeBuilder) Named(name string) Builder {
	b.delegate = b.delegate.Named(name)
	return b
}

func (b *runtimeBuilder) For(obj client.Object, opts ...builder.ForOption) Builder {
	b.delegate = b.delegate.For(obj, opts...)
	return b
}

func (b *runtimeBuilder) Owns(obj client.Object, opts ...builder.OwnsOption) Builder {
	b.delegate = b.delegate.Owns(obj, opts...)
	return b
}

func (b *runtimeBuilder) WatchesRawSource(src source.Source) Builder {
	b.delegate = b.delegate.WatchesRawSource(src)
	return b
}

func (b *runtimeBuilder) WithOptions(opts controller.Options) Builder {
	b.delegate = b.delegate.WithOptions(opts)
	return b
}

func (b *runtimeBuilder) Complete(r reconcile.Reconciler) error {
	return b.delegate.Complete(r)
}

