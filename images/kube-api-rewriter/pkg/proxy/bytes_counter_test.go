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
	"io"
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

func TestBytesCounterWriter(t *testing.T) {
	var dst bytes.Buffer
	writer := BytesCounterWriterWrap(&dst)
	if _, err := writer.Write([]byte("hello")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if CounterValue(writer) != 5 {
		t.Fatalf("unexpected counter value: %d", CounterValue(writer))
	}

	if _, err := io.WriteString(writer, "world"); err != nil {
		t.Fatalf("write string failed: %v", err)
	}
	if CounterValue(writer) != 10 {
		t.Fatalf("expected cumulative counter, got: %d", CounterValue(writer))
	}

	CounterReset(writer)
	if CounterValue(writer) != 0 {
		t.Fatalf("expected reset writer counter")
	}
	if dst.String() != "helloworld" {
		t.Fatalf("unexpected dst content: %s", dst.String())
	}
}
