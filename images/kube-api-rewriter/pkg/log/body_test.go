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

package log

import (
	"bytes"
	"io"
	"testing"
)

func TestReaderLogger(t *testing.T) {
	src := bytes.NewBufferString("abcdef")
	r := NewReaderLogger(src)

	dst := &bytes.Buffer{}
	if _, err := io.Copy(dst, r); err != nil {
		t.Fatalf("copy failed: %v", err)
	}

	if HeadString(r, 10) != "abcdef" {
		t.Fatalf("unexpected head string")
	}
	if HeadString(r, 3) != "abc" {
		t.Fatalf("unexpected truncated head")
	}
	if HeadStringEx(r, 4) != "[4] abcd" {
		t.Fatalf("unexpected head ex: %s", HeadStringEx(r, 4))
	}
	if !HasData(r) {
		t.Fatalf("expected data")
	}
	if got := string(Bytes(r)); got != "abcdef" {
		t.Fatalf("unexpected bytes: %q", got)
	}

	if HeadString("wrong", 10) != "" {
		t.Fatalf("expected empty head for foreign type")
	}
	if HeadStringEx("wrong", 1) != "<empty>" {
		t.Fatalf("expected empty head ex for foreign type")
	}
	if HasData("wrong") {
		t.Fatalf("expected false for foreign type")
	}
	if Bytes("wrong") != nil {
		t.Fatalf("expected nil for foreign type")
	}

	if err := r.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}
