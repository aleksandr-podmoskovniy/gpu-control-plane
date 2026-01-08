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

import "github.com/aleksandr-podmoskovniy/gpu/pkg/common/patch"

func (r *Resource[T, ST]) jsonPatchOpsForFinalizers() []patch.JSONPatchOperation {
	current := r.currentObj.GetFinalizers()
	desired := r.changedObj.GetFinalizers()
	switch {
	case len(current) == 0 && len(desired) == 0:
		return nil
	case len(current) == 0 && len(desired) > 0:
		return []patch.JSONPatchOperation{patch.WithAdd("/metadata/finalizers", desired)}
	case len(current) > 0 && len(desired) == 0:
		return []patch.JSONPatchOperation{patch.WithRemove("/metadata/finalizers")}
	default:
		return []patch.JSONPatchOperation{patch.WithReplace("/metadata/finalizers", desired)}
	}
}

func (r *Resource[T, ST]) jsonPatchOpsForAnnotations() []patch.JSONPatchOperation {
	return jsonPatchOpsForMap("/metadata/annotations", r.currentObj.GetAnnotations(), r.changedObj.GetAnnotations())
}

func (r *Resource[T, ST]) jsonPatchOpsForLabels() []patch.JSONPatchOperation {
	return jsonPatchOpsForMap("/metadata/labels", r.currentObj.GetLabels(), r.changedObj.GetLabels())
}

func jsonPatchOpsForMap(path string, current, desired map[string]string) []patch.JSONPatchOperation {
	switch {
	case current == nil && desired == nil:
		return nil
	case current == nil && desired != nil:
		return []patch.JSONPatchOperation{patch.WithAdd(path, desired)}
	case current != nil && desired == nil:
		return []patch.JSONPatchOperation{patch.WithRemove(path)}
	}

	ops := []patch.JSONPatchOperation{
		patch.NewJSONPatchOperation(patch.PatchTestOp, path, current),
	}

	for key := range current {
		if _, ok := desired[key]; ok {
			continue
		}
		ops = append(ops, patch.WithRemove(path+"/"+patch.EscapeJSONPointer(key)))
	}

	for key, value := range desired {
		currentValue, ok := current[key]
		pathKey := path + "/" + patch.EscapeJSONPointer(key)
		switch {
		case !ok:
			ops = append(ops, patch.WithAdd(pathKey, value))
		case currentValue != value:
			ops = append(ops, patch.WithReplace(pathKey, value))
		}
	}

	return ops
}
