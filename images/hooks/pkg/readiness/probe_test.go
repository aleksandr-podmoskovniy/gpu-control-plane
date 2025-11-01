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

package readiness

import (
	"context"
	"testing"

	"github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"

	"hooks/pkg/settings"
)

func newInput(t *testing.T, values map[string]any) *pkg.HookInput {
	t.Helper()

	pv, err := patchablevalues.NewPatchableValues(values)
	if err != nil {
		t.Fatalf("create patchable values: %v", err)
	}

	return &pkg.HookInput{Values: pv}
}

func newInputWithValidation(t *testing.T, validation any) *pkg.HookInput {
	if validation == nil {
		return newInput(t, map[string]any{})
	}

	return newInput(t, map[string]any{
		settings.ConfigRoot: map[string]any{
			"internal": map[string]any{
				"moduleConfigValidation": validation,
			},
		},
	})
}

func TestCheckModuleReadinessReady(t *testing.T) {
	input := newInputWithValidation(t, nil)

	if err := checkModuleReadiness(context.Background(), input); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckModuleReadinessValidationError(t *testing.T) {
	input := newInputWithValidation(t, map[string]any{"error": "failure"})

	err := checkModuleReadiness(context.Background(), input)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "failure" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckModuleReadinessPending(t *testing.T) {
	input := newInputWithValidation(t, map[string]any{})

	if err := checkModuleReadiness(context.Background(), input); err == nil || err.Error() != "module is not ready yet" {
		t.Fatalf("expected pending error, got %v", err)
	}
}

func TestCheckModuleReadinessMalformed(t *testing.T) {
	input := newInputWithValidation(t, true)

	if err := checkModuleReadiness(context.Background(), input); err == nil || err.Error() != "module validation state is malformed" {
		t.Fatalf("expected malformed error, got %v", err)
	}
}
