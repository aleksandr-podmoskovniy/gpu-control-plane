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

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
)

func TestNewGPUDeviceWatcher(t *testing.T) {
	if NewGPUDeviceWatcher(testr.New(t)) == nil {
		t.Fatalf("expected watcher instance")
	}
}

func TestGPUDeviceWatcherEnqueueBranches(t *testing.T) {
	w := NewGPUDeviceWatcher(testr.New(t))

	t.Run("nil-device", func(t *testing.T) {
		if got := w.enqueue(context.Background(), nil); got != nil {
			t.Fatalf("expected nil requests, got %+v", got)
		}
	})

	t.Run("nodeName-from-status", func(t *testing.T) {
		reqs := w.enqueue(context.Background(), &v1alpha1.GPUDevice{Status: v1alpha1.GPUDeviceStatus{NodeName: " node-a "}})
		if len(reqs) != 1 || reqs[0].Name != "node-a" {
			t.Fatalf("unexpected requests: %+v", reqs)
		}
	})

	t.Run("no-nodeName", func(t *testing.T) {
		if got := w.enqueue(context.Background(), &v1alpha1.GPUDevice{}); got != nil {
			t.Fatalf("expected nil requests, got %+v", got)
		}
	})
}

func TestGPUDeviceWatcherWatchBranches(t *testing.T) {
	w := NewGPUDeviceWatcher(testr.New(t))

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
