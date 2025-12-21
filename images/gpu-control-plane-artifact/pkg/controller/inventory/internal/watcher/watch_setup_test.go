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
	"errors"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestNodeFeatureWatcherWatchBranches(t *testing.T) {
	w := NewNodeFeatureWatcher()

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

func TestNodeWatcherWatchBranches(t *testing.T) {
	w := NewNodeWatcher()

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

func TestNodeWatcherPredicatesNilCreateObject(t *testing.T) {
	p := nodePredicates()
	if p.Create(event.TypedCreateEvent[*corev1.Node]{Object: nil}) {
		t.Fatalf("expected create to ignore nil node")
	}
}

func TestNewNodeFeatureWatcherAndNewNodeWatcher(t *testing.T) {
	if NewNodeFeatureWatcher() == nil {
		t.Fatalf("expected watcher instance")
	}
	if NewNodeWatcher() == nil {
		t.Fatalf("expected watcher instance")
	}

	// cover testr logger init in unrelated code paths as a sanity check
	_ = testr.New(t)
	_ = (&corev1.Node{ObjectMeta: metav1.ObjectMeta{}}).DeepCopy()
}

