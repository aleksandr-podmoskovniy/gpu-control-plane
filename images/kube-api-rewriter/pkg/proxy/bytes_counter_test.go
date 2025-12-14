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

package proxy

import (
	"bytes"
	"testing"
)

func TestBytesCounterReader(t *testing.T) {
	reader := BytesCounterReaderWrap(bytes.NewBufferString("abcdef"))
	buf := make([]byte, 3)
	if _, err := reader.Read(buf); err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if CounterValue(reader) != 3 {
		t.Fatalf("unexpected counter value: %d", CounterValue(reader))
	}

	CounterReset(reader)
	if CounterValue(reader) != 0 {
		t.Fatalf("expected counter reset")
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

func TestBytesCounterHelpers_NonCounterNoop(t *testing.T) {
	CounterReset(struct{}{})
	if CounterValue(struct{}{}) != 0 {
		t.Fatalf("expected CounterValue to return 0 for non-counter")
	}
}
