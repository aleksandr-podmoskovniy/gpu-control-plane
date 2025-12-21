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

package watchers

import (
	"errors"
	"testing"

	"github.com/go-logr/logr/testr"
)

func TestWatchSetupBranches(t *testing.T) {
	t.Run("gpu-device-gpupool", func(t *testing.T) {
		w := NewGPUPoolGPUDeviceWatcher(testr.New(t))

		if err := w.Watch(&stubManager{cache: nil}, &stubController{}); err == nil {
			t.Fatalf("expected cache required error")
		}
		if err := w.Watch(&stubManager{cache: &fakeCache{}, client: &noopClient{}}, &stubController{err: errors.New("watch fail")}); err == nil {
			t.Fatalf("expected watch error")
		}

		ctr := &stubController{}
		if err := w.Watch(&stubManager{cache: &fakeCache{}, client: &noopClient{}}, ctr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ctr.watched) != 1 {
			t.Fatalf("expected 1 watch, got %d", len(ctr.watched))
		}
		if w.cl == nil {
			t.Fatalf("expected manager client to be captured")
		}
	})

	t.Run("gpu-device-cluster", func(t *testing.T) {
		w := NewClusterGPUPoolGPUDeviceWatcher(testr.New(t))
		if err := w.Watch(&stubManager{cache: nil}, &stubController{}); err == nil {
			t.Fatalf("expected cache required error")
		}
		ctr := &stubController{}
		if err := w.Watch(&stubManager{cache: &fakeCache{}}, ctr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ctr.watched) != 1 {
			t.Fatalf("expected 1 watch, got %d", len(ctr.watched))
		}
	})

	t.Run("validator-pod-gpupool", func(t *testing.T) {
		w := NewGPUPoolValidatorPodWatcher(testr.New(t))
		if err := w.Watch(&stubManager{cache: nil}, &stubController{}); err == nil {
			t.Fatalf("expected cache required error")
		}
		ctr := &stubController{}
		if err := w.Watch(&stubManager{cache: &fakeCache{}, client: &noopClient{}}, ctr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ctr.watched) != 1 {
			t.Fatalf("expected 1 watch, got %d", len(ctr.watched))
		}
		if w.cl == nil {
			t.Fatalf("expected manager client to be captured")
		}
	})

	t.Run("validator-pod-cluster", func(t *testing.T) {
		w := NewClusterGPUPoolValidatorPodWatcher(testr.New(t))
		if err := w.Watch(&stubManager{cache: nil}, &stubController{}); err == nil {
			t.Fatalf("expected cache required error")
		}
		ctr := &stubController{}
		if err := w.Watch(&stubManager{cache: &fakeCache{}}, ctr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ctr.watched) != 1 {
			t.Fatalf("expected 1 watch, got %d", len(ctr.watched))
		}
	})
}

