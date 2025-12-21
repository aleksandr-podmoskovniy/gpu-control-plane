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

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/bootstrap/meta"
)

func TestNewWorkloadPodWatcher(t *testing.T) {
	w := NewWorkloadPodWatcher(testr.New(t))
	for _, name := range meta.ComponentAppNames() {
		if _, ok := w.managedAppSet[name]; !ok {
			t.Fatalf("expected managed app %q to be present", name)
		}
	}
}

func TestWorkloadPodWatcherEnqueueBranches(t *testing.T) {
	w := NewWorkloadPodWatcher(testr.New(t))

	t.Run("nil-pod", func(t *testing.T) {
		if got := w.enqueue(context.Background(), nil); got != nil {
			t.Fatalf("expected nil requests, got %+v", got)
		}
	})

	base := &corev1.Pod{}

	t.Run("wrong-namespace", func(t *testing.T) {
		pod := base.DeepCopy()
		pod.Namespace = "other"
		if got := w.enqueue(context.Background(), pod); got != nil {
			t.Fatalf("expected nil requests, got %+v", got)
		}
	})

	t.Run("missing-nodeName", func(t *testing.T) {
		pod := base.DeepCopy()
		pod.Namespace = meta.WorkloadsNamespace
		if got := w.enqueue(context.Background(), pod); got != nil {
			t.Fatalf("expected nil requests, got %+v", got)
		}
	})

	t.Run("missing-labels", func(t *testing.T) {
		pod := base.DeepCopy()
		pod.Namespace = meta.WorkloadsNamespace
		pod.Spec.NodeName = "node-a"
		if got := w.enqueue(context.Background(), pod); got != nil {
			t.Fatalf("expected nil requests, got %+v", got)
		}
	})

	t.Run("unmanaged-app", func(t *testing.T) {
		pod := base.DeepCopy()
		pod.Namespace = meta.WorkloadsNamespace
		pod.Spec.NodeName = "node-a"
		pod.Labels = map[string]string{"app": "other"}
		if got := w.enqueue(context.Background(), pod); got != nil {
			t.Fatalf("expected nil requests, got %+v", got)
		}
	})

	t.Run("managed-app", func(t *testing.T) {
		pod := base.DeepCopy()
		pod.Namespace = meta.WorkloadsNamespace
		pod.Spec.NodeName = "node-a"
		pod.Labels = map[string]string{"app": meta.ComponentAppNames()[0]}
		reqs := w.enqueue(context.Background(), pod)
		if len(reqs) != 1 || reqs[0].Name != "node-a" {
			t.Fatalf("unexpected requests: %+v", reqs)
		}
	})
}

func TestWorkloadPodWatcherWatchBranches(t *testing.T) {
	w := NewWorkloadPodWatcher(testr.New(t))

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
