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
	"strings"
	"testing"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

type stubHandler struct {
	name  string
	calls *[]string
	err   error
}

func (h stubHandler) Name() string {
	return h.name
}

func (h stubHandler) Handle(_ context.Context, _ state.State) error {
	*h.calls = append(*h.calls, h.name)
	return h.err
}

func TestStepsStop(t *testing.T) {
	log := logger.NewLogger("info", "discard", 0)
	calls := make([]string, 0, 3)
	steps := NewSteps(
		log,
		stubHandler{name: "first", calls: &calls},
		stubHandler{name: "stop", calls: &calls, err: StopHandlerChain(errors.New("halt"))},
		stubHandler{name: "after", calls: &calls},
	)

	_, err := steps.Run(context.Background(), state.New("node-1"))
	if err != nil {
		t.Fatalf("expected stop to be handled without error: %v", err)
	}
	if len(calls) != 2 || calls[0] != "first" || calls[1] != "stop" {
		t.Fatalf("unexpected handler order: %v", calls)
	}
}

func TestStepsError(t *testing.T) {
	log := logger.NewLogger("info", "discard", 0)
	calls := make([]string, 0, 2)
	steps := NewSteps(
		log,
		stubHandler{name: "first", calls: &calls, err: errors.New("boom")},
		stubHandler{name: "after", calls: &calls},
	)

	_, err := steps.Run(context.Background(), state.New("node-1"))
	if err == nil {
		t.Fatalf("expected error from handler")
	}
	if !strings.Contains(err.Error(), "first") {
		t.Fatalf("expected handler name in error, got: %v", err)
	}
	if len(calls) != 1 || calls[0] != "first" {
		t.Fatalf("unexpected handler order: %v", calls)
	}
}
