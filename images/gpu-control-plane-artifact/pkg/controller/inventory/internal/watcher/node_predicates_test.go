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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestNodePredicates(t *testing.T) {
	preds := nodePredicates()

	if !preds.Create(event.TypedCreateEvent[*corev1.Node]{Object: &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de", "gpu.deckhouse.io/device.00.device": "1db5", "gpu.deckhouse.io/device.00.class": "0300"}}}}) {
		t.Fatalf("expected create predicate true for GPU labels")
	}
	oldNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de"}}}
	newNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"another": "label"}}}
	if !preds.Update(event.TypedUpdateEvent[*corev1.Node]{ObjectOld: oldNode, ObjectNew: newNode}) {
		t.Fatalf("expected update predicate true when old node has GPU labels")
	}
	if !preds.Delete(event.TypedDeleteEvent[*corev1.Node]{}) {
		t.Fatalf("expected delete predicate always true")
	}
	if preds.Generic(event.TypedGenericEvent[*corev1.Node]{}) {
		t.Fatalf("expected generic predicate false")
	}

	// same GPU labels should not requeue
	sameOld := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de"}}}
	sameNew := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de"}}}
	if preds.Update(event.TypedUpdateEvent[*corev1.Node]{ObjectOld: sameOld, ObjectNew: sameNew}) {
		t.Fatalf("expected update with unchanged GPU labels to be filtered out")
	}

	// adding GPU labels should trigger
	noLabels := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}}}
	withLabels := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"gpu.deckhouse.io/device.00.vendor": "10de"}}}
	if !preds.Update(event.TypedUpdateEvent[*corev1.Node]{ObjectOld: noLabels, ObjectNew: withLabels}) {
		t.Fatalf("expected update adding GPU labels to trigger")
	}
}
