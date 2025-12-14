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

package inventory

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/event"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestNodeFeaturePredicates(t *testing.T) {
	pred := nodeFeaturePredicates()

	withGPU := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Labels: map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de"},
		},
	}
	noGPU := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Labels: map[string]string{"other": "label"},
		},
	}

	if !pred.Create(event.TypedCreateEvent[*nfdv1alpha1.NodeFeature]{Object: withGPU}) {
		t.Fatalf("expected create with GPU labels to pass")
	}
	if pred.Create(event.TypedCreateEvent[*nfdv1alpha1.NodeFeature]{Object: noGPU}) {
		t.Fatalf("expected create without GPU labels to be filtered out")
	}

	if pred.Update(event.TypedUpdateEvent[*nfdv1alpha1.NodeFeature]{ObjectOld: noGPU, ObjectNew: noGPU}) {
		t.Fatalf("expected update without GPU labels to be ignored")
	}

	// unchanged GPU labels should not trigger
	if pred.Update(event.TypedUpdateEvent[*nfdv1alpha1.NodeFeature]{ObjectOld: withGPU, ObjectNew: withGPU.DeepCopy()}) {
		t.Fatalf("expected update with identical GPU labels to be ignored")
	}

	// changing GPU label should trigger
	changed := withGPU.DeepCopy()
	changed.Spec.Labels["gpu.deckhouse.io/device.00.vendor"] = "abcd"
	if !pred.Update(event.TypedUpdateEvent[*nfdv1alpha1.NodeFeature]{ObjectOld: withGPU, ObjectNew: changed}) {
		t.Fatalf("expected update with changed GPU labels to trigger")
	}

	// dropping GPU labels should trigger
	if !pred.Update(event.TypedUpdateEvent[*nfdv1alpha1.NodeFeature]{ObjectOld: withGPU, ObjectNew: noGPU}) {
		t.Fatalf("expected update removing GPU labels to trigger")
	}

	if !pred.Delete(event.TypedDeleteEvent[*nfdv1alpha1.NodeFeature]{Object: withGPU}) {
		t.Fatalf("expected delete with GPU labels to trigger")
	}
	if pred.Delete(event.TypedDeleteEvent[*nfdv1alpha1.NodeFeature]{Object: noGPU}) {
		t.Fatalf("expected delete without GPU labels to be ignored")
	}
	if pred.Generic(event.TypedGenericEvent[*nfdv1alpha1.NodeFeature]{}) {
		t.Fatalf("expected generic to be ignored")
	}
}
