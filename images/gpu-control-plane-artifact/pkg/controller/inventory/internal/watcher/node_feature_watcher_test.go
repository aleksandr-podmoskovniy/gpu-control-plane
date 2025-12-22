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

package watcher

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestMapNodeFeatureToNode(t *testing.T) {
	if reqs := mapNodeFeatureToNode(context.Background(), nil); reqs != nil {
		t.Fatalf("expected nil feature to return nil, got %v", reqs)
	}

	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-d"},
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Labels: map[string]string{
				gfdProductLabel: "NVIDIA A100",
			},
		},
	}
	reqs := mapNodeFeatureToNode(context.Background(), feature)
	if len(reqs) != 1 || reqs[0].Name != "worker-d" {
		t.Fatalf("unexpected requests: %+v", reqs)
	}

	noName := &nfdv1alpha1.NodeFeature{}
	if reqs := mapNodeFeatureToNode(context.Background(), noName); reqs != nil {
		t.Fatalf("expected empty requests, got %+v", reqs)
	}

	noGPU := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-e"},
	}
	if reqs := mapNodeFeatureToNode(context.Background(), noGPU); reqs != nil {
		t.Fatalf("expected empty requests for feature without GPU labels, got %+v", reqs)
	}

	prefixed := feature.DeepCopy()
	prefixed.Name = "nvidia-features-for-worker-x"
	reqs = mapNodeFeatureToNode(context.Background(), prefixed)
	if len(reqs) != 1 || reqs[0].Name != "worker-x" {
		t.Fatalf("expected trimmed node name, got %+v", reqs)
	}

	emptyTrim := feature.DeepCopy()
	emptyTrim.Name = "nvidia-features-for-"
	if reqs := mapNodeFeatureToNode(context.Background(), emptyTrim); reqs != nil {
		t.Fatalf("expected nil requests when trimmed name is empty, got %+v", reqs)
	}

	labeled := feature.DeepCopy()
	labeled.ObjectMeta = metav1.ObjectMeta{
		Name:   "some-generated-name",
		Labels: map[string]string{nodeFeatureNodeNameLabel: "worker-from-label"},
	}
	reqs = mapNodeFeatureToNode(context.Background(), labeled)
	if len(reqs) != 1 || reqs[0].Name != "worker-from-label" {
		t.Fatalf("expected node name from label, got %+v", reqs)
	}
}

func TestNodeFeatureLabelsNil(t *testing.T) {
	if nodeFeatureLabels(nil) != nil {
		t.Fatalf("expected nodeFeatureLabels(nil) to return nil")
	}
}

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
