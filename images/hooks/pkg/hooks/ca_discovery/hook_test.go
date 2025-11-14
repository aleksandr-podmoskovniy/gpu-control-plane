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

package ca_discovery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse/pkg/log"

	pkg "github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"
	"github.com/deckhouse/module-sdk/pkg/utils"

	"hooks/pkg/settings"
)

type staticSnapshots struct {
	items map[string][]pkg.Snapshot
}

func (s staticSnapshots) Get(key string) []pkg.Snapshot {
	return s.items[key]
}

type jsonSnapshot struct {
	value any
	err   error
}

func (s jsonSnapshot) UnmarshalTo(target any) error {
	if s.err != nil {
		return s.err
	}
	raw, err := json.Marshal(s.value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func (s jsonSnapshot) String() string {
	raw, _ := json.Marshal(s.value)
	return string(raw)
}

func newInput(t *testing.T, snapshots map[string][]pkg.Snapshot) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	return newInputWithValues(t, snapshots, map[string]any{})
}

func newInputWithValues(t *testing.T, snapshots map[string][]pkg.Snapshot, initial map[string]any) (*pkg.HookInput, *patchablevalues.PatchableValues) {
	t.Helper()

	if initial == nil {
		initial = map[string]any{}
	}

	values, err := patchablevalues.NewPatchableValues(initial)
	if err != nil {
		t.Fatalf("create patchable values: %v", err)
	}

	return &pkg.HookInput{
		Values:    values,
		Snapshots: staticSnapshots{items: snapshots},
		Logger:    log.NewNop(),
	}, values
}

func TestHandleModuleCommonCANoSecret(t *testing.T) {
	input, patches := newInput(t, map[string][]pkg.Snapshot{})

	if err := handleModuleCommonCA(context.Background(), input); err != nil {
		t.Fatalf("handleModuleCommonCA returned error: %v", err)
	}

	if patch := lastPatchForPath(patches.GetPatches(), patchPath(settings.InternalRootCAPath)); patch != nil {
		t.Fatalf("expected no patch for %s when secret is absent", settings.InternalRootCAPath)
	}
}

func TestHandleModuleCommonCAUpdatesValues(t *testing.T) {
	crtPEM := "-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----"
	keyPEM := "-----BEGIN PRIVATE KEY-----\nFAKE\n-----END PRIVATE KEY-----"

	secret := caSecret{
		Crt: []byte(crtPEM),
		Key: []byte(keyPEM),
	}

	input, patches := newInput(t, map[string][]pkg.Snapshot{
		commonCASecretSnapshot: {
			jsonSnapshot{value: secret},
		},
	})

	if err := handleModuleCommonCA(context.Background(), input); err != nil {
		t.Fatalf("handleModuleCommonCA returned error: %v", err)
	}

	patch := lastPatchForPath(patches.GetPatches(), patchPath(settings.InternalRootCAPath))
	if patch == nil {
		t.Fatalf("expected patch for %s", settings.InternalRootCAPath)
	}

	var payload map[string]string
	if err := json.Unmarshal(patch.Value, &payload); err != nil {
		t.Fatalf("unmarshal patch value: %v", err)
	}

	if payload["crt"] != crtPEM {
		t.Fatalf("unexpected crt payload: %q", payload["crt"])
	}
	if payload["key"] != keyPEM {
		t.Fatalf("unexpected key payload: %q", payload["key"])
	}
}

func TestHandleModuleCommonCAPropagatesErrors(t *testing.T) {
	input, _ := newInput(t, map[string][]pkg.Snapshot{
		commonCASecretSnapshot: {
			jsonSnapshot{err: errors.New("boom")},
		},
	})

	if err := handleModuleCommonCA(context.Background(), input); err == nil {
		t.Fatal("expected error when snapshot decoding fails")
	}
}

func TestHandleModuleCommonCARemovesValuesWhenSecretEmpty(t *testing.T) {
	initial := map[string]any{
		settings.ModuleValuesName: map[string]any{
			"internal": map[string]any{
				"rootCA": map[string]any{
					"crt": "stale",
				},
			},
		},
	}

	input, patches := newInputWithValues(t, map[string][]pkg.Snapshot{
		commonCASecretSnapshot: {
			jsonSnapshot{value: caSecret{}},
		},
	}, initial)

	if err := handleModuleCommonCA(context.Background(), input); err != nil {
		t.Fatalf("handleModuleCommonCA returned error: %v", err)
	}

	patch := lastPatchForPath(patches.GetPatches(), patchPath(settings.InternalRootCAPath))
	if patch == nil {
		t.Fatalf("expected removal patch for %s", settings.InternalRootCAPath)
	}
	if patch.Op != "remove" {
		t.Fatalf("expected remove operation, got %s", patch.Op)
	}
}

func patchPath(path string) string {
	return "/" + strings.ReplaceAll(path, ".", "/")
}

func lastPatchForPath(patches []*utils.ValuesPatchOperation, path string) *utils.ValuesPatchOperation {
	var result *utils.ValuesPatchOperation
	for _, patch := range patches {
		if patch.Path == path {
			result = patch
		}
	}
	return result
}

func TestSecretJSONRoundTrip(t *testing.T) {
	crt := "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----"
	key := "-----BEGIN KEY-----\nTEST\n-----END"

	payload := caSecret{Crt: []byte(crt), Key: []byte(key)}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded caSecret
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(decoded.Crt) != crt || string(decoded.Key) != key {
		t.Fatalf("unexpected roundtrip: crt=%q key=%q", decoded.Crt, decoded.Key)
	}
}
