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

package reconciler

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/patch"
)

// Update writes status changes and patches metadata changes.
func (r *Resource[T, ST]) Update(ctx context.Context) error {
	if r.IsEmpty() {
		return nil
	}

	if !reflect.DeepEqual(r.getObjStatus(r.currentObj), r.getObjStatus(r.changedObj)) {
		finalizers := r.changedObj.GetFinalizers()
		labels := r.changedObj.GetLabels()
		annotations := r.changedObj.GetAnnotations()
		if err := r.client.Status().Update(ctx, r.changedObj); err != nil {
			return fmt.Errorf("error updating status subresource: %w", err)
		}
		r.changedObj.SetFinalizers(finalizers)
		r.changedObj.SetLabels(labels)
		r.changedObj.SetAnnotations(annotations)
	}

	metadataPatch := patch.NewJSONPatch()

	if !slices.Equal(r.currentObj.GetFinalizers(), r.changedObj.GetFinalizers()) {
		metadataPatch.Append(r.jsonPatchOpsForFinalizers()...)
	}
	if !maps.Equal(r.currentObj.GetAnnotations(), r.changedObj.GetAnnotations()) {
		metadataPatch.Append(r.jsonPatchOpsForAnnotations()...)
	}
	if !maps.Equal(r.currentObj.GetLabels(), r.changedObj.GetLabels()) {
		metadataPatch.Append(r.jsonPatchOpsForLabels()...)
	}

	if metadataPatch.Len() == 0 {
		return nil
	}

	metadataPatchBytes, err := metadataPatch.Bytes()
	if err != nil {
		return err
	}
	jsonPatch := client.RawPatch(types.JSONPatchType, metadataPatchBytes)
	if err = r.client.Patch(ctx, r.changedObj, jsonPatch); err != nil {
		if r.changedObj.GetDeletionTimestamp() != nil && len(r.changedObj.GetFinalizers()) == 0 && kerrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("error patching metadata (%s, ops=%d): %w", string(metadataPatchBytes), len(metadataPatch.Operations()), err)
	}

	return nil
}
