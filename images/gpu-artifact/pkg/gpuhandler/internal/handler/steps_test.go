/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
)

type trackingHandler struct {
	name     string
	onHandle func() error
	calls    *int
}

func (h trackingHandler) Name() string { return h.name }

func (h trackingHandler) Handle(_ context.Context, _ state.State) error {
	if h.calls != nil {
		*h.calls++
	}
	if h.onHandle != nil {
		return h.onHandle()
	}
	return nil
}

func TestStepsStopsChain(t *testing.T) {
	var firstCalls, secondCalls int
	steps := NewSteps(log.NewNop(),
		trackingHandler{name: "stopper", calls: &firstCalls, onHandle: func() error { return ErrStopHandlerChain }},
		trackingHandler{name: "after", calls: &secondCalls},
	)

	if _, err := steps.Run(context.Background(), state.New("node-1")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if firstCalls != 1 {
		t.Fatalf("expected first handler to be called once, got %d", firstCalls)
	}
	if secondCalls != 0 {
		t.Fatalf("expected second handler to be skipped, got %d", secondCalls)
	}
}

func TestStepsPropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	steps := NewSteps(log.NewNop(),
		trackingHandler{name: "fail", onHandle: func() error { return wantErr }},
	)

	_, err := steps.Run(context.Background(), state.New("node-1"))
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error to wrap wantErr, got %v", err)
	}
}
