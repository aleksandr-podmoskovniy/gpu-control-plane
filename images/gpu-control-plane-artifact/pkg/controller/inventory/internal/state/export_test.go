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

package state

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"
)

func TestExportedBuilders(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "Node 1",
		Labels: map[string]string{"a": "b"},
	}}
	feature := &nfdv1alpha1.NodeFeature{
		Spec: nfdv1alpha1.NodeFeatureSpec{Labels: map[string]string{"c": "d"}},
	}
	policy := ManagedNodesPolicy{LabelKey: "gpu.deckhouse.io/managed", EnabledByDefault: true}

	snapshot := BuildNodeSnapshot(node, feature, policy)
	if !snapshot.FeatureDetected || !snapshot.Managed {
		t.Fatalf("unexpected snapshot %+v", snapshot)
	}
	if snapshot.Labels["a"] != "b" || snapshot.Labels["c"] != "d" {
		t.Fatalf("unexpected labels: %+v", snapshot.Labels)
	}

	dev := DeviceSnapshot{Index: "0", Vendor: "10de", Device: "20b0"}
	if got := BuildDeviceName("Node 1", dev); got != "node-1-0-10de-20b0" {
		t.Fatalf("unexpected device name: %q", got)
	}
	if got := BuildInventoryID("Node 1", dev); got != "node-1-0-10de-20b0" {
		t.Fatalf("unexpected inventory id: %q", got)
	}
}
