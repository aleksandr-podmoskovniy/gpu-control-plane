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

package renderer

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type listErrorClient struct {
	client.Client
	err error
}

func (c listErrorClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return c.err
}

type createNthErrorClient struct {
	client.Client
	failOn int
	err    error

	calls int
}

func (c *createNthErrorClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.calls++
	if c.calls == c.failOn {
		return c.err
	}
	return c.Client.Create(ctx, obj, opts...)
}

type deleteNthErrorClient struct {
	client.Client
	failOn int
	err    error

	calls int
}

func (c *deleteNthErrorClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	c.calls++
	if c.calls == c.failOn {
		return c.err
	}
	return c.Client.Delete(ctx, obj, opts...)
}
