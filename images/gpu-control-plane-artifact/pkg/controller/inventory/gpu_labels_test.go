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

func TestNodeFeatureMapSkipsEmptyNameAfterPrefixTrim(t *testing.T) {
	feature := &nfdv1alpha1.NodeFeature{
		ObjectMeta: metav1.ObjectMeta{Name: "nvidia-features-for-"},
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Labels: map[string]string{
				"gpu.deckhouse.io/device.00.vendor": "10de",
			},
		},
	}
	if reqs := mapNodeFeatureToNode(context.Background(), feature); reqs != nil {
		t.Fatalf("expected empty request list when node name is empty, got %v", reqs)
	}
}

func TestNodeFeatureLabelsNil(t *testing.T) {
	if nodeFeatureLabels(nil) != nil {
		t.Fatalf("expected nodeFeatureLabels(nil) to return nil")
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

func TestGpuLabelDifferenceDetection(t *testing.T) {
	base := map[string]string{
		"gpu.deckhouse.io/device.00.vendor": "10de",
		"nvidia.com/mig-1g.10gb.count":      "1",
	}
	same := map[string]string{
		"gpu.deckhouse.io/device.00.vendor": "10de",
		"nvidia.com/mig-1g.10gb.count":      "1",
	}
	if gpuLabelsDiffer(base, same) {
		t.Fatalf("identical GPU labels should not differ")
	}

	changedValue := map[string]string{
		"gpu.deckhouse.io/device.00.vendor": "abcd",
	}
	if !gpuLabelsDiffer(base, changedValue) {
		t.Fatalf("changed GPU labels should be detected")
	}

	addedKey := map[string]string{
		"gpu.deckhouse.io/device.00.vendor": "10de",
		"nvidia.com/mig-strategy":           "single",
	}
	if !gpuLabelsDiffer(base, addedKey) {
		t.Fatalf("added relevant GPU label should be detected")
	}

	irrelevantChange := map[string]string{
		"gpu.deckhouse.io/device.00.vendor": "10de",
		"nvidia.com/mig-1g.10gb.count":      "1",
		"unrelated":                         "x",
	}
	if gpuLabelsDiffer(base, irrelevantChange) {
		t.Fatalf("irrelevant label changes should be ignored")
	}

	if !gpuNodeLabelsChanged(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: base}}, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: changedValue}}) {
		t.Fatalf("gpuNodeLabelsChanged should detect GPU label diff")
	}
	if gpuNodeLabelsChanged(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: base}}, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: irrelevantChange}}) {
		t.Fatalf("gpuNodeLabelsChanged should ignore irrelevant changes")
	}
}

func TestGpuLabelsDifferCoversBranches(t *testing.T) {
	base := map[string]string{
		gfdMigAltStrategy: "single",
		"unrelated":       "x",
	}
	if gpuLabelsDiffer(base, map[string]string{
		gfdMigAltStrategy: "single",
		"unrelated":       "x",
	}) {
		t.Fatalf("expected identical labels to not differ")
	}

	// Cover nil-map read fallback.
	if !gpuLabelsDiffer(map[string]string{gfdMigAltStrategy: "single"}, nil) {
		t.Fatalf("expected nil newLabels to differ")
	}

	// Ensure differences introduced only in newLabels are detected in the second loop.
	if !gpuLabelsDiffer(
		map[string]string{gfdMigAltStrategy: "single"},
		map[string]string{gfdMigAltStrategy: "single", gfdProductLabel: "A100"},
	) {
		t.Fatalf("expected additional relevant key in newLabels to be detected")
	}
}
