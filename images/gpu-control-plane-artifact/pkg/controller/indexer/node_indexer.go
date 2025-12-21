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

package indexer

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func IndexNodeByTaintKey() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &corev1.Node{}, NodeTaintKeyField, func(object client.Object) []string {
		node, ok := object.(*corev1.Node)
		if !ok || len(node.Spec.Taints) == 0 {
			return nil
		}
		seen := make(map[string]struct{}, len(node.Spec.Taints))
		keys := make([]string, 0, len(node.Spec.Taints))
		for _, taint := range node.Spec.Taints {
			if taint.Key == "" {
				continue
			}
			if _, ok := seen[taint.Key]; ok {
				continue
			}
			seen[taint.Key] = struct{}{}
			keys = append(keys, taint.Key)
		}
		if len(keys) == 0 {
			return nil
		}
		return keys
	}
}
