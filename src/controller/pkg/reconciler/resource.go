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

package reconciler

import (
	"context"
	"errors"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errNilClient   = errors.New("resource client is not configured")
	errNilResource = errors.New("resource is not initialized")
)

// Resource tracks original and mutated objects to produce minimal patches.
type Resource[T client.Object] struct {
	original T
	current  T
	client   client.Client
}

func isNilObject[T any](obj T) bool {
	val := reflect.ValueOf(obj)
	if !val.IsValid() {
		return true
	}
	switch val.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Slice:
		return val.IsNil()
	default:
		return false
	}
}

// NewResource clones the provided object and prepares it for patch operations.
func NewResource[T client.Object](obj T, cl client.Client) *Resource[T] {
	var original T
	if !isNilObject(obj) {
		if copy, ok := obj.DeepCopyObject().(T); ok {
			original = copy
		}
	}
	return &Resource[T]{original: original, current: obj, client: cl}
}

// Original returns the snapshot captured at creation time. Do not mutate it.
func (r *Resource[T]) Original() T {
	return r.original
}

// PatchStatus applies status changes via the status subresource using MergeFrom.
func (r *Resource[T]) PatchStatus(ctx context.Context) error {
	if r.client == nil {
		return errNilClient
	}
	if isNilObject(r.current) || isNilObject(r.original) {
		return errNilResource
	}
	return r.client.Status().Patch(ctx, r.current, client.MergeFrom(r.original))
}
