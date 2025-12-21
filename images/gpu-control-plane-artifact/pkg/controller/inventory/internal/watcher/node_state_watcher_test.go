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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestNewNodeStateWatcher(t *testing.T) {
	if NewNodeStateWatcher() == nil {
		t.Fatalf("expected watcher instance")
	}
}

func TestMapNodeStateToNode(t *testing.T) {
	if got := mapNodeStateToNode(context.Background(), nil); got != nil {
		t.Fatalf("expected nil requests, got %+v", got)
	}
	if got := mapNodeStateToNode(context.Background(), &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: " "}}); got != nil {
		t.Fatalf("expected nil requests for empty name, got %+v", got)
	}
	reqs := mapNodeStateToNode(context.Background(), &v1alpha1.GPUNodeState{ObjectMeta: metav1.ObjectMeta{Name: " node-a "}})
	if len(reqs) != 1 || reqs[0].Name != "node-a" {
		t.Fatalf("unexpected requests: %+v", reqs)
	}
}

func TestNodeStatePredicates(t *testing.T) {
	p := nodeStatePredicates()
	if p.Create(event.TypedCreateEvent[*v1alpha1.GPUNodeState]{Object: &v1alpha1.GPUNodeState{}}) {
		t.Fatalf("expected create to be ignored")
	}
	if p.Update(event.TypedUpdateEvent[*v1alpha1.GPUNodeState]{ObjectOld: &v1alpha1.GPUNodeState{}, ObjectNew: &v1alpha1.GPUNodeState{}}) {
		t.Fatalf("expected update to be ignored")
	}
	if !p.Delete(event.TypedDeleteEvent[*v1alpha1.GPUNodeState]{Object: &v1alpha1.GPUNodeState{}}) {
		t.Fatalf("expected delete to trigger")
	}
	if p.Generic(event.TypedGenericEvent[*v1alpha1.GPUNodeState]{Object: &v1alpha1.GPUNodeState{}}) {
		t.Fatalf("expected generic to be ignored")
	}
}

func TestNodeStateWatcherWatchBranches(t *testing.T) {
	w := NewNodeStateWatcher()

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
