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
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"testing"

	"github.com/tidwall/gjson"
)

func TestCopyHeader_Branches(t *testing.T) {
	dst := http.Header{"X-Test": []string{"keep"}}
	src := http.Header{"X-Test": []string{"override"}, "X-Add": []string{"v1", "v2"}}

	copyHeader(dst, src)

	if got := dst.Get("X-Test"); got != "keep" {
		t.Fatalf("expected dst header to keep original value, got %q", got)
	}
	if got := dst.Values("X-Add"); len(got) != 2 || got[0] != "v1" || got[1] != "v2" {
		t.Fatalf("unexpected dst added values: %#v", got)
	}
}

func TestEncodingAwareReaderWrap_Branches(t *testing.T) {
	t.Run("passthrough for unknown encoding", func(t *testing.T) {
		orig := io.NopCloser(bytes.NewBufferString("x"))
		reader, err := encodingAwareReaderWrap(orig, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if reader != orig {
			t.Fatalf("expected passthrough reader")
		}
	})

	t.Run("gzip decode", func(t *testing.T) {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write([]byte("hello"))
		_ = zw.Close()

		reader, err := encodingAwareReaderWrap(io.NopCloser(bytes.NewReader(buf.Bytes())), "gzip")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Cleanup(func() { reader.Close() })

		got, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(got) != "hello" {
			t.Fatalf("unexpected decoded content: %q", string(got))
		}
	})

	t.Run("gzip decode error", func(t *testing.T) {
		_, err := encodingAwareReaderWrap(io.NopCloser(bytes.NewBufferString("not-gzip")), "gzip")
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("deflate decode", func(t *testing.T) {
		var buf bytes.Buffer
		zw, _ := flate.NewWriter(&buf, flate.DefaultCompression)
		_, _ = zw.Write([]byte("hello"))
		_ = zw.Close()

		reader, err := encodingAwareReaderWrap(io.NopCloser(bytes.NewReader(buf.Bytes())), "deflate")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Cleanup(func() { reader.Close() })

		got, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(got) != "hello" {
			t.Fatalf("unexpected decoded content: %q", string(got))
		}
	})
}

func TestNotFoundJSON(t *testing.T) {
	res := notFoundJSON("pods", []byte(`{"metadata":{"name":"p1"}}`))
	if gjson.GetBytes(res, "code").Int() != 404 {
		t.Fatalf("expected code 404, got %s", gjson.GetBytes(res, "code").Raw)
	}
	if gjson.GetBytes(res, "reason").String() != "NotFound" {
		t.Fatalf("expected NotFound reason")
	}
	if gjson.GetBytes(res, "details.kind").String() != "pods" {
		t.Fatalf("unexpected details.kind")
	}
	if gjson.GetBytes(res, "details.name").String() != "p1" {
		t.Fatalf("unexpected details.name")
	}
}

type flushingWriter struct {
	bytes.Buffer
	flushed int
}

func (w *flushingWriter) Flush() {
	w.flushed++
}

func TestImmediateWriter_WriteBranches(t *testing.T) {
	t.Run("chunkFn and flusher", func(t *testing.T) {
		dst := &flushingWriter{}
		chunkCalls := 0
		iw := &immediateWriter{
			dst: dst,
			chunkFn: func([]byte) {
				chunkCalls++
			},
		}

		_, _ = iw.Write([]byte("a"))
		if chunkCalls != 1 {
			t.Fatalf("expected chunkFn to be called")
		}
		if dst.flushed != 1 {
			t.Fatalf("expected Flush to be called")
		}
	})

	t.Run("no chunkFn, no flusher", func(t *testing.T) {
		var dst bytes.Buffer
		iw := &immediateWriter{dst: &dst}
		_, _ = iw.Write([]byte("a"))
		if dst.String() != "a" {
			t.Fatalf("unexpected output: %q", dst.String())
		}
	})
}
