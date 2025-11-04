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

package prepare_internal

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	pkg "github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"
	"github.com/deckhouse/module-sdk/pkg/utils"
	"github.com/deckhouse/module-sdk/testing/mock"

	"hooks/pkg/settings"
)

func TestHandleInitializesInternalTree(t *testing.T) {
	values := map[string]any{}
	input, pv := newHookInput(t, nil, values)

	if err := handle(context.Background(), input); err != nil {
		t.Fatalf("handle returned error: %v", err)
	}

	expectPatch(t, pv, "add", "/"+settings.ModuleValuesName)
	expectPatch(t, pv, "add", "/"+settings.ModuleValuesName+"/internal")
	expectPatch(t, pv, "add", "/"+settings.ModuleValuesName+"/internal/rootCA")
	expectPatch(t, pv, "add", "/"+settings.ModuleValuesName+"/internal/controller/cert")
}

func TestHandleUnmarshalError(t *testing.T) {
	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(key string) []pkg.Snapshot {
		if key != moduleSnapshot {
			t.Fatalf("unexpected snapshot key: %s", key)
		}
		snap := mock.NewSnapshotMock(t)
		snap.UnmarshalToMock.Set(func(target any) error {
			return assertAnError()
		})
		return []pkg.Snapshot{snap}
	})

	values := map[string]any{}
	input, _ := newHookInput(t, snapshots, values)

	if err := handle(context.Background(), input); err == nil {
		t.Fatalf("expected error from handle")
	}
}

func assertAnError() error {
	return fmt.Errorf("boom")
}

func TestHandleSkipsExistingInternal(t *testing.T) {
	values := map[string]any{
		settings.ModuleValuesName: map[string]any{
			"internal": map[string]any{
				"rootCA":     map[string]any{"existing": true},
				"controller": map[string]any{"cert": map[string]any{"cert": true}},
			},
		},
	}

	input, pv := newHookInput(t, nil, values)
	if err := handle(context.Background(), input); err != nil {
		t.Fatalf("handle returned error: %v", err)
	}

	for _, p := range pv.GetPatches() {
		if p.Op == "add" && (p.Path == "/"+settings.ModuleValuesName || p.Path == "/"+settings.ModuleValuesName+"/internal" ||
			p.Path == "/"+settings.ModuleValuesName+"/internal/rootCA" || p.Path == "/"+settings.ModuleValuesName+"/internal/controller/cert") {
			t.Fatalf("unexpected patch for existing path: %#v", p)
		}
	}
}

func TestHandleSetsModuleConfigEnabled(t *testing.T) {
	snapshots := mock.NewSnapshotsMock(t)
	snapshots.GetMock.Set(func(key string) []pkg.Snapshot {
		if key != moduleSnapshot {
			t.Fatalf("unexpected snapshot key: %s", key)
		}
		snap := mock.NewSnapshotMock(t)
		snap.UnmarshalToMock.Set(func(target any) error {
			payload := map[string]any{"enabled": true}
			data, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			return json.Unmarshal(data, target)
		})
		return []pkg.Snapshot{snap}
	})

	values := map[string]any{}
	input, pv := newHookInput(t, snapshots, values)

	if err := handle(context.Background(), input); err != nil {
		t.Fatalf("handle returned error: %v", err)
	}

	op := expectPatch(t, pv, "add", "/"+settings.ModuleValuesName+"/internal/moduleConfig")
	var payload map[string]any
	if err := json.Unmarshal(op.Value, &payload); err != nil {
		t.Fatalf("decode moduleConfig payload: %v", err)
	}
	if payload["enabled"] != true {
		t.Fatalf("expected enabled true, got %v", payload["enabled"])
	}
}

func newHookInput(t *testing.T, snapshots pkg.Snapshots, values map[string]any) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	t.Helper()

	if snapshots == nil {
		s := mock.NewSnapshotsMock(t)
		s.GetMock.Set(func(string) []pkg.Snapshot {
			return nil
		})
		snapshots = s
	}

	pv, err := patchablevalues.NewPatchableValues(values)
	if err != nil {
		t.Fatalf("create patchable values: %v", err)
	}

	input := &pkg.HookInput{
		Snapshots: snapshots,
		Values:    pv,
	}
	return input, pv
}

func expectPatch(t *testing.T, pv *patchablevalues.PatchableValues, op, path string) *utils.ValuesPatchOperation {
	t.Helper()
	for _, p := range pv.GetPatches() {
		if p.Op == op && p.Path == path {
			return p
		}
	}
	t.Fatalf("expected patch op=%s path=%s, got %#v", op, path, pv.GetPatches())
	return nil
}
