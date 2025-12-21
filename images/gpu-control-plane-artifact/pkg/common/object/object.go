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

package object

import (
	"context"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

func FetchObject[T client.Object](ctx context.Context, key types.NamespacedName, cl client.Client, obj T, opts ...client.GetOption) (T, error) {
	if err := cl.Get(ctx, key, obj, opts...); err != nil {
		var empty T
		if apierrors.IsNotFound(err) {
			return empty, nil
		}
		return empty, err
	}
	return obj, nil
}

func DeleteObject[T client.Object](ctx context.Context, cl client.Client, obj T, opts ...client.DeleteOption) error {
	if isNilObject(obj) || obj.GetDeletionTimestamp() != nil {
		return nil
	}
	err := cl.Delete(ctx, obj, opts...)
	if err != nil && apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
