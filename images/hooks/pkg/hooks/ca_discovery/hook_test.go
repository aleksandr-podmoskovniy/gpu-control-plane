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
	"encoding/base64"
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
	t.Helper()

	values, err := patchablevalues.NewPatchableValues(map[string]any{})
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
		Crt: base64.StdEncoding.EncodeToString([]byte(crtPEM)),
		Key: base64.StdEncoding.EncodeToString([]byte(keyPEM)),
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

func TestHandleModuleCommonCAHandlesPlainValues(t *testing.T) {
	secret := caSecret{Crt: "plain-cert", Key: "plain-key"}

	input, patches := newInput(t, map[string][]pkg.Snapshot{
		commonCASecretSnapshot: {jsonSnapshot{value: secret}},
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

	if payload["crt"] != "plain-cert" || payload["key"] != "plain-key" {
		t.Fatalf("unexpected payload: %#v", payload)
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

func TestDecodeMaybeBase64(t *testing.T) {
	crt := "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----"
	crtB64 := base64.StdEncoding.EncodeToString([]byte(crt))
	pem := "-----BEGIN KEY-----\nTEST\n-----END"
	pemB64 := base64.StdEncoding.EncodeToString([]byte(pem))
	rawHello := base64.StdEncoding.EncodeToString([]byte("hello"))

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "pem", input: crtB64, want: crt},
		{name: "pem with bare end marker", input: pemB64, want: pem},
		{name: "plain text base64", input: rawHello, want: "hello"},
		{name: "invalid base64 falls back", input: "!!!not-base64!!!", want: "!!!not-base64!!!"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := decodeMaybeBase64(tc.input); got != tc.want {
				t.Fatalf("decodeMaybeBase64(%q)=%q want %q", tc.input, got, tc.want)
			}
		})
	}
}
