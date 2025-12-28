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

package prepare

import (
	"context"
	"errors"
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

type fakeStore struct {
	calls *[]string
	err   error
}

func (f *fakeStore) Save(_ context.Context, _ domain.PrepareRequest) error {
	*f.calls = append(*f.calls, "checkpoint")
	return f.err
}

type fakeWriter struct {
	name  string
	calls *[]string
	err   error
}

func (f *fakeWriter) Write(_ context.Context, _ domain.PrepareRequest) error {
	*f.calls = append(*f.calls, f.name)
	return f.err
}

func TestRunOnceOrder(t *testing.T) {
	t.Parallel()

	var calls []string
	service := NewService(
		&fakeStore{calls: &calls},
		&fakeWriter{name: "cdi", calls: &calls},
		&fakeWriter{name: "hook", calls: &calls},
	)

	if err := service.RunOnce(context.Background(), domain.PrepareRequest{}); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	want := []string{"checkpoint", "cdi", "hook"}
	if len(calls) != len(want) {
		t.Fatalf("expected %d calls, got %d: %v", len(want), len(calls), calls)
	}
	for i, item := range want {
		if calls[i] != item {
			t.Fatalf("expected call %d=%s, got %s", i, item, calls[i])
		}
	}
}

func TestRunOnceStopsOnError(t *testing.T) {
	t.Parallel()

	var calls []string
	storeErr := errors.New("store failed")
	service := NewService(
		&fakeStore{calls: &calls, err: storeErr},
		&fakeWriter{name: "cdi", calls: &calls},
		&fakeWriter{name: "hook", calls: &calls},
	)

	if err := service.RunOnce(context.Background(), domain.PrepareRequest{}); !errors.Is(err, storeErr) {
		t.Fatalf("expected error %v, got %v", storeErr, err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected only checkpoint call, got %v", calls)
	}
}
