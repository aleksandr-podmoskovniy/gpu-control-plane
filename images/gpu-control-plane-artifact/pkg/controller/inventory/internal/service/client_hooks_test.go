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

package service

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type hookClient struct {
	client.Client

	get    func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error
	list   func(context.Context, client.ObjectList, ...client.ListOption) error
	create func(context.Context, client.Object, ...client.CreateOption) error
	patch  func(context.Context, client.Object, client.Patch, ...client.PatchOption) error
	delete func(context.Context, client.Object, ...client.DeleteOption) error

	status client.StatusWriter
}

func (h *hookClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if h.get != nil {
		return h.get(ctx, key, obj, opts...)
	}
	return h.Client.Get(ctx, key, obj, opts...)
}

func (h *hookClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if h.list != nil {
		return h.list(ctx, list, opts...)
	}
	return h.Client.List(ctx, list, opts...)
}

func (h *hookClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if h.create != nil {
		return h.create(ctx, obj, opts...)
	}
	return h.Client.Create(ctx, obj, opts...)
}

func (h *hookClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if h.patch != nil {
		return h.patch(ctx, obj, patch, opts...)
	}
	return h.Client.Patch(ctx, obj, patch, opts...)
}

func (h *hookClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if h.delete != nil {
		return h.delete(ctx, obj, opts...)
	}
	return h.Client.Delete(ctx, obj, opts...)
}

func (h *hookClient) Status() client.StatusWriter {
	if h.status != nil {
		return h.status
	}
	return h.Client.Status()
}

type hookStatusWriter struct {
	base client.StatusWriter

	create func(context.Context, client.Object, client.Object, ...client.SubResourceCreateOption) error
	update func(context.Context, client.Object, ...client.SubResourceUpdateOption) error
	patch  func(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error
}

func (h hookStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	if h.create != nil {
		return h.create(ctx, obj, subResource, opts...)
	}
	return h.base.Create(ctx, obj, subResource, opts...)
}

func (h hookStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if h.update != nil {
		return h.update(ctx, obj, opts...)
	}
	return h.base.Update(ctx, obj, opts...)
}

func (h hookStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	if h.patch != nil {
		return h.patch(ctx, obj, patch, opts...)
	}
	return h.base.Patch(ctx, obj, patch, opts...)
}
