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

package dra

import (
	"context"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/publisher"
)

// Runtime bundles DRA-related services.
type Runtime struct {
	allocator *allocator.Service
	publisher *publisher.Service
	preparer  *prepare.Service
}

// NewRuntime creates a DRA runtime bundle.
func NewRuntime(alloc *allocator.Service, pub *publisher.Service, prep *prepare.Service) *Runtime {
	return &Runtime{
		allocator: alloc,
		publisher: pub,
		preparer:  prep,
	}
}

// RunAllocator executes a single allocator cycle.
func (r *Runtime) RunAllocator(ctx context.Context) error {
	if r.allocator == nil {
		return nil
	}
	return r.allocator.RunOnce(ctx)
}

// RunPublisher executes a single publish cycle.
func (r *Runtime) RunPublisher(ctx context.Context, resourceExists bool) error {
	if r.publisher == nil {
		return nil
	}
	return r.publisher.PublishOnce(ctx, resourceExists)
}

// RunPrepare executes a single prepare cycle.
func (r *Runtime) RunPrepare(ctx context.Context) error {
	if r.preparer == nil {
		return nil
	}
	return r.preparer.RunOnce(ctx, prepareRequest)
}

var prepareRequest = prepare.DefaultRequest()
