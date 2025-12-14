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

package proxy

import (
	"bytes"
	"testing"
)

func TestCreateWatchDecoder_Branches(t *testing.T) {
	t.Run("ParseMediaType error", func(t *testing.T) {
		_, err := createWatchDecoder(bytes.NewReader(nil), "application/json; charset")
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("empty content type uses default serializer", func(t *testing.T) {
		dec, err := createWatchDecoder(bytes.NewReader(nil), "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dec == nil {
			t.Fatalf("expected decoder")
		}
		if err := dec.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	})

	t.Run("media type with nil StreamSerializer", func(t *testing.T) {
		_, err := createWatchDecoder(bytes.NewReader(nil), "application/yaml")
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}
