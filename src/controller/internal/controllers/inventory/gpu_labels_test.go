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
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestNodeFeatureMapUsesInstances(t *testing.T) {
	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Features: nfdv1alpha1.Features{
				Instances: map[string]nfdv1alpha1.InstanceFeatureSet{},
			},
			Labels: map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de", "gpu.deckhouse.io/device.00.device": "1db5", "gpu.deckhouse.io/device.00.class": "0300"},
		},
	}

	reqs := mapNodeFeatureToNode(context.Background(), feature)
	if len(reqs) != 1 || reqs[0] != (reconcile.Request{NamespacedName: types.NamespacedName{Name: "worker-1"}}) {
		t.Fatalf("expected one reconcile request for worker-1, got %v", reqs)
	}
}

func TestNodeLabelsHelpers(t *testing.T) {
	if nodeLabels(nil) != nil {
		t.Fatalf("nodeLabels(nil) should return nil")
	}
	lbls := nodeLabels(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "b"}}})
	if lbls["a"] != "b" {
		t.Fatalf("nodeLabels should return labels map")
	}
	if nodeHasGPUHardwareLabels(map[string]string{}) {
		t.Fatalf("empty labels should not be treated as GPU")
	}
	if nodeHasGPUHardwareLabels(map[string]string{}) {
		t.Fatalf("empty labels should not be treated as GPU")
	}
	labels := map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de", "gpu.deckhouse.io/device.00.device": "1db5", "gpu.deckhouse.io/device.00.class": "0300"}
	if !nodeHasGPUHardwareLabels(labels) {
		t.Fatalf("expected GPU labels to be detected")
	}
	if !nodeHasGPUHardwareLabels(map[string]string{"nvidia.com/mig-1g.10gb.count": "1"}) {
		t.Fatalf("expected MIG labels to trigger GPU detection")
	}
	if !nodeHasGPUHardwareLabels(map[string]string{"nvidia.com/mig-strategy": "mixed"}) {
		t.Fatalf("expected MIG strategy labels to trigger GPU detection")
	}
	if !nodeHasGPUHardwareLabels(map[string]string{"nvidia.com/gpu.product": "A100"}) {
		t.Fatalf("expected product labels to trigger GPU detection")
	}
	if !nodeHasGPUHardwareLabels(map[string]string{"nvidia.com/gpu.memory": "40960"}) {
		t.Fatalf("expected memory labels to trigger GPU detection")
	}
	if !nodeHasGPUHardwareLabels(map[string]string{"nvidia.com/mig-capable": "true"}) {
		t.Fatalf("expected mig-capable labels to trigger GPU detection")
	}
	if !nodeHasGPUHardwareLabels(map[string]string{"nvidia.com/mig-strategy": "single"}) {
		t.Fatalf("expected mig strategy labels to trigger GPU detection")
	}
	if !nodeHasGPUHardwareLabels(map[string]string{"nvidia.com/mig.capable": "true"}) {
		t.Fatalf("expected alt mig-capable labels to trigger GPU detection")
	}
	if !nodeHasGPUHardwareLabels(map[string]string{"nvidia.com/mig.strategy": "mixed"}) {
		t.Fatalf("expected alt mig strategy labels to trigger GPU detection")
	}
	if !nodeHasGPUHardwareLabels(map[string]string{"nvidia.com/mig-strategy": "alt"}) {
		t.Fatalf("expected mig alt strategy labels to trigger GPU detection")
	}
	if !nodeHasGPUHardwareLabels(map[string]string{"nvidia.com/mig-capable": "true"}) {
		t.Fatalf("expected mig alt capable labels to trigger GPU detection")
	}
	if !nodeHasGPUHardwareLabels(map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de"}) {
		t.Fatalf("expected device label prefix to trigger GPU detection")
	}
	if !nodeHasGPUHardwareLabels(map[string]string{"nvidia.com/mig-1g.10gb.count": "1"}) {
		t.Fatalf("expected mig profile prefix to trigger GPU detection")
	}
	if nodeHasGPUHardwareLabels(map[string]string{"foo": "bar"}) {
		t.Fatalf("unexpected detection for unrelated labels")
	}
	if nodeHasGPUHardwareLabels(nil) {
		t.Fatalf("unexpected detection for nil labels")
	}
}
