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

package allocator

import (
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func consumedCapacity(in map[string]resource.Quantity) map[resourcev1.QualifiedName]resource.Quantity {
	if len(in) == 0 {
		return nil
	}
	out := make(map[resourcev1.QualifiedName]resource.Quantity, len(in))
	for key, val := range in {
		out[resourcev1.QualifiedName(key)] = val.DeepCopy()
	}
	return out
}

func nodeSelectorForNode(nodeName string) *corev1.NodeSelector {
	if nodeName == "" {
		return nil
	}
	return &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{{
			MatchFields: []corev1.NodeSelectorRequirement{{
				Key:      "metadata.name",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{nodeName},
			}},
		}},
	}
}
