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
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	common "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common"
)

func TestNewGFDPodWatcher(t *testing.T) {
	w := NewGFDPodWatcher()
	if w.gfdApp != common.AppName(common.ComponentGPUFeatureDiscovery) {
		t.Fatalf("unexpected gfd app label: %q", w.gfdApp)
	}
}

func TestMapGFDPodToNode(t *testing.T) {
	if got := mapGFDPodToNode(context.Background(), nil); got != nil {
		t.Fatalf("expected nil requests, got %+v", got)
	}

	pod := &corev1.Pod{Spec: corev1.PodSpec{NodeName: " node-a "}}
	reqs := mapGFDPodToNode(context.Background(), pod)
	if len(reqs) != 1 || reqs[0].Name != "node-a" {
		t.Fatalf("unexpected requests: %+v", reqs)
	}

	pod.Spec.NodeName = ""
	if got := mapGFDPodToNode(context.Background(), pod); got != nil {
		t.Fatalf("expected nil requests for empty nodeName, got %+v", got)
	}
}

func TestIsGFDPod(t *testing.T) {
	gfdApp := common.AppName(common.ComponentGPUFeatureDiscovery)

	if isGFDPod(nil, gfdApp) {
		t.Fatalf("expected nil pod to not match")
	}
	if isGFDPod(&corev1.Pod{}, gfdApp) {
		t.Fatalf("expected pod without labels to not match")
	}
	if isGFDPod(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "other", Labels: map[string]string{"app": gfdApp}}}, gfdApp) {
		t.Fatalf("expected wrong namespace to not match")
	}
	if isGFDPod(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: common.WorkloadsNamespace, Labels: map[string]string{"app": "other"}}}, gfdApp) {
		t.Fatalf("expected wrong app label to not match")
	}
	if !isGFDPod(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: common.WorkloadsNamespace, Labels: map[string]string{"app": gfdApp}}}, gfdApp) {
		t.Fatalf("expected matching GFD pod")
	}
}

func TestIsPodReady(t *testing.T) {
	if isPodReady(nil) {
		t.Fatalf("expected nil pod to not be ready")
	}
	if isPodReady(&corev1.Pod{}) {
		t.Fatalf("expected pod without conditions to not be ready")
	}
	pod := &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	if !isPodReady(pod) {
		t.Fatalf("expected ready condition to be detected")
	}
	pod.Status.Conditions[0].Status = corev1.ConditionFalse
	if isPodReady(pod) {
		t.Fatalf("expected not-ready condition to be detected")
	}
}

func TestGFDPodPredicatesBranches(t *testing.T) {
	gfdApp := common.AppName(common.ComponentGPUFeatureDiscovery)
	p := gfdPodPredicates(gfdApp)

	readyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: common.WorkloadsNamespace, Labels: map[string]string{"app": gfdApp}},
		Spec:       corev1.PodSpec{NodeName: "node-a"},
		Status: corev1.PodStatus{
			PodIP:      "10.0.0.1",
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}

	if !p.Create(event.TypedCreateEvent[*corev1.Pod]{Object: readyPod}) {
		t.Fatalf("expected ready GFD pod create to pass")
	}
	notReady := readyPod.DeepCopy()
	notReady.Status.Conditions[0].Status = corev1.ConditionFalse
	if p.Create(event.TypedCreateEvent[*corev1.Pod]{Object: notReady}) {
		t.Fatalf("expected non-ready GFD pod create to be filtered")
	}

	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: readyPod, ObjectNew: nil}) {
		t.Fatalf("expected update with nil new pod to pass through")
	}
	if p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: readyPod, ObjectNew: &corev1.Pod{}}) {
		t.Fatalf("expected update for non-gfd pod to be ignored")
	}
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: nil, ObjectNew: readyPod}) {
		t.Fatalf("expected update with nil old pod to trigger")
	}

	oldOther := readyPod.DeepCopy()
	oldOther.Labels["app"] = "other"
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: oldOther, ObjectNew: readyPod}) {
		t.Fatalf("expected update from non-gfd to gfd to trigger")
	}

	oldDifferentNode := readyPod.DeepCopy()
	oldDifferentNode.Spec.NodeName = "node-b"
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: oldDifferentNode, ObjectNew: readyPod}) {
		t.Fatalf("expected nodeName change to trigger")
	}

	oldDifferentIP := readyPod.DeepCopy()
	oldDifferentIP.Status.PodIP = "10.0.0.2"
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: oldDifferentIP, ObjectNew: readyPod}) {
		t.Fatalf("expected podIP change to trigger")
	}

	oldNotReady := readyPod.DeepCopy()
	oldNotReady.Status.Conditions[0].Status = corev1.ConditionFalse
	if !p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: oldNotReady, ObjectNew: readyPod}) {
		t.Fatalf("expected readiness change to trigger")
	}
	if p.Update(event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: readyPod, ObjectNew: readyPod.DeepCopy()}) {
		t.Fatalf("expected unchanged pod to be ignored")
	}

	if p.Delete(event.TypedDeleteEvent[*corev1.Pod]{Object: readyPod}) {
		t.Fatalf("expected delete to be ignored")
	}
	if p.Generic(event.TypedGenericEvent[*corev1.Pod]{Object: readyPod}) {
		t.Fatalf("expected generic to be ignored")
	}
}

func TestGFDPodWatcherWatchBranches(t *testing.T) {
	w := NewGFDPodWatcher()

	t.Run("requires-cache", func(t *testing.T) {
		err := w.Watch(&stubManager{cache: nil}, &stubController{})
		if err == nil {
			t.Fatalf("expected cache required error")
		}
	})

	t.Run("propagates-watch-error", func(t *testing.T) {
		err := w.Watch(&stubManager{cache: &fakeCache{}}, &stubController{err: errors.New("watch fail")})
		if err == nil {
			t.Fatalf("expected watch error")
		}
	})

	t.Run("registers-watch", func(t *testing.T) {
		ctr := &stubController{}
		if err := w.Watch(&stubManager{cache: &fakeCache{}}, ctr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ctr.watched) != 1 {
			t.Fatalf("expected 1 watch registration, got %d", len(ctr.watched))
		}
	})
}
