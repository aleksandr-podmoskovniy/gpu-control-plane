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
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/tidwall/gjson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/rewriter"
)

type fakeDecoder struct {
	results []decodeResult
	idx     int
}

type decodeResult struct {
	// If fill is non-nil and into is *metav1.WatchEvent, fill is copied into into.
	fill *metav1.WatchEvent
	// If returnInto is true, Decode returns the into object.
	returnInto bool
	// Otherwise Decode returns ret (may be nil).
	ret runtime.Object
	err error
}

func (d *fakeDecoder) Decode(_ *schema.GroupVersionKind, into runtime.Object) (runtime.Object, *schema.GroupVersionKind, error) {
	if d.idx >= len(d.results) {
		return nil, nil, io.EOF
	}
	r := d.results[d.idx]
	d.idx++

	if r.fill != nil {
		if we, ok := into.(*metav1.WatchEvent); ok {
			*we = *r.fill
		}
	}

	if r.returnInto {
		return into, nil, r.err
	}

	return r.ret, nil, r.err
}

func (d *fakeDecoder) Close() error { return nil }

type writeErrFlusher struct {
	writes  int
	flushed int
}

func (w *writeErrFlusher) Write(p []byte) (int, error) {
	w.writes++
	return 0, errors.New("write failed")
}

func (w *writeErrFlusher) Flush() {
	w.flushed++
}

func TestStreamRewriter_transformWatchEvent_Branches(t *testing.T) {
	rwr := newSimpleRewriter(t)
	req, _ := http.NewRequest(http.MethodGet, "https://kubernetes/apis/original.group.io/v1/someresources?watch=true", nil)
	targetReq := rewriter.NewTargetRequest(rwr, req)

	s := &streamRewriter{
		targetReq: targetReq,
		rewriter:  rwr,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		metrics:   NewProxyMetrics(context.Background(), newStubMetricsProvider()),
	}

	t.Run("unknown WatchEvent type", func(t *testing.T) {
		_, err := s.transformWatchEvent(&metav1.WatchEvent{
			Type: "???",
			Object: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"original.group.io/v1","kind":"SomeResource"}`),
			},
		})
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing apiVersion and kind", func(t *testing.T) {
		_, err := s.transformWatchEvent(&metav1.WatchEvent{
			Type: string(watch.Added),
			Object: runtime.RawExtension{
				Raw: []byte(`{}`),
			},
		})
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("bookmark uses RestoreBookmark", func(t *testing.T) {
		ev, err := s.transformWatchEvent(&metav1.WatchEvent{
			Type: string(watch.Bookmark),
			Object: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource","metadata":{"resourceVersion":"1"}}`),
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ev == nil {
			t.Fatalf("expected rewritten event")
		}
		if gjson.GetBytes(ev.Object.Raw, "apiVersion").String() != "original.group.io/v1" {
			t.Fatalf("expected restored apiVersion")
		}
		if gjson.GetBytes(ev.Object.Raw, "kind").String() != "SomeResource" {
			t.Fatalf("expected restored kind")
		}
	})

	t.Run("SkipItem is returned as-is", func(t *testing.T) {
		ev, err := s.transformWatchEvent(&metav1.WatchEvent{
			Type: string(watch.Added),
			Object: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"original.group.io/v1","kind":"SomeResource"}`),
			},
		})
		if !errors.Is(err, rewriter.SkipItem) {
			t.Fatalf("expected SkipItem, got %v", err)
		}
		if ev != nil {
			t.Fatalf("expected nil event")
		}
	})

	t.Run("non-SkipItem rewrite error is wrapped", func(t *testing.T) {
		_, err := s.transformWatchEvent(&metav1.WatchEvent{
			Type: string(watch.Added),
			Object: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinition","metadata":{"name":"malformed"}}`),
			},
		})
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("restores object in event", func(t *testing.T) {
		ev, err := s.transformWatchEvent(&metav1.WatchEvent{
			Type: string(watch.Added),
			Object: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource"}`),
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ev == nil {
			t.Fatalf("expected event")
		}
		if gjson.GetBytes(ev.Object.Raw, "apiVersion").String() != "original.group.io/v1" {
			t.Fatalf("expected restored apiVersion")
		}
	})
}

func TestStreamRewriter_writeEvent_Branches(t *testing.T) {
	t.Run("marshal error on invalid RawExtension", func(t *testing.T) {
		dst := &bytes.Buffer{}
		s := &streamRewriter{
			dst:     dst,
			log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
			metrics: NewProxyMetrics(context.Background(), newStubMetricsProvider()),
		}
		s.writeEvent(&metav1.WatchEvent{
			Type: string(watch.Added),
			Object: runtime.RawExtension{
				Raw: []byte("{"),
			},
		})
		if dst.Len() != 0 {
			t.Fatalf("expected no output on marshal error")
		}
	})

	t.Run("write error and flush", func(t *testing.T) {
		dst := &writeErrFlusher{}
		s := &streamRewriter{
			dst:     dst,
			log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
			metrics: NewProxyMetrics(context.Background(), newStubMetricsProvider()),
		}
		s.writeEvent(&metav1.WatchEvent{
			Type: string(watch.Added),
			Object: runtime.RawExtension{
				Raw: []byte(`{}`),
			},
		})
		if dst.writes != 1 {
			t.Fatalf("expected dst.Write to be called")
		}
		if dst.flushed != 1 {
			t.Fatalf("expected Flush to be called")
		}
	})

	t.Run("write success and flush", func(t *testing.T) {
		dst := &flushingWriter{}
		s := &streamRewriter{
			dst:     dst,
			log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
			metrics: NewProxyMetrics(context.Background(), newStubMetricsProvider()),
		}
		s.writeEvent(&metav1.WatchEvent{
			Type: string(watch.Added),
			Object: runtime.RawExtension{
				Raw: []byte(`{}`),
			},
		})
		if dst.Len() == 0 {
			t.Fatalf("expected bytes written")
		}
		if dst.flushed != 1 {
			t.Fatalf("expected Flush to be called")
		}
	})
}

func TestStreamRewriter_start_Branches(t *testing.T) {
	rwr := newSimpleRewriter(t)
	req, _ := http.NewRequest(http.MethodGet, "https://kubernetes/apis/original.group.io/v1/someresources?watch=true", nil)
	targetReq := rewriter.NewTargetRequest(rwr, req)

	base := func(dec streaming.Decoder, dst io.Writer, done chan struct{}) *streamRewriter {
		return &streamRewriter{
			dst:          dst,
			bytesCounter: BytesCounterReaderWrap(bytes.NewReader(nil)),
			src:          io.NopCloser(bytes.NewReader(nil)),
			rewriter:     rwr,
			targetReq:    targetReq,
			decoder:      dec,
			done:         done,
			log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
			metrics:      NewProxyMetrics(context.Background(), newStubMetricsProvider()),
		}
	}

	t.Run("returns on canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		d := &fakeDecoder{results: []decodeResult{{
			fill:       &metav1.WatchEvent{Type: string(watch.Added), Object: runtime.RawExtension{Raw: []byte(`{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource"}`)}},
			returnInto: true,
		}}}
		s := base(d, &bytes.Buffer{}, make(chan struct{}))
		s.start(ctx)
	})

	t.Run("handles decoder EOF", func(t *testing.T) {
		d := &fakeDecoder{results: []decodeResult{{returnInto: true, err: io.EOF}}}
		s := base(d, &bytes.Buffer{}, make(chan struct{}))
		s.start(context.Background())
	})

	t.Run("handles decoder unexpected EOF", func(t *testing.T) {
		d := &fakeDecoder{results: []decodeResult{{returnInto: true, err: io.ErrUnexpectedEOF}}}
		s := base(d, &bytes.Buffer{}, make(chan struct{}))
		s.start(context.Background())
	})

	t.Run("handles decoder generic error", func(t *testing.T) {
		d := &fakeDecoder{results: []decodeResult{{returnInto: true, err: errors.New("boom")}}}
		s := base(d, &bytes.Buffer{}, make(chan struct{}))
		s.start(context.Background())
	})

	t.Run("handles decoder probable EOF error", func(t *testing.T) {
		d := &fakeDecoder{results: []decodeResult{{returnInto: true, err: errors.New("connection reset by peer")}}}
		s := base(d, &bytes.Buffer{}, make(chan struct{}))
		s.start(context.Background())
	})

	t.Run("handles res != &got and then stops", func(t *testing.T) {
		d := &fakeDecoder{results: []decodeResult{
			{ret: &metav1.WatchEvent{}, err: nil},
			{returnInto: true, err: io.EOF},
		}}
		s := base(d, &bytes.Buffer{}, make(chan struct{}))
		s.start(context.Background())
	})

	t.Run("handles SkipItem without writing", func(t *testing.T) {
		d := &fakeDecoder{results: []decodeResult{
			{
				fill:       &metav1.WatchEvent{Type: string(watch.Added), Object: runtime.RawExtension{Raw: []byte(`{"apiVersion":"original.group.io/v1","kind":"SomeResource"}`)}},
				returnInto: true,
			},
			{returnInto: true, err: io.EOF},
		}}
		dst := &bytes.Buffer{}
		s := base(d, dst, make(chan struct{}))
		s.start(context.Background())
		if dst.Len() != 0 {
			t.Fatalf("expected no output")
		}
	})

	t.Run("writes original event on transform error", func(t *testing.T) {
		d := &fakeDecoder{results: []decodeResult{
			{
				fill:       &metav1.WatchEvent{Type: string(watch.Added), Object: runtime.RawExtension{Raw: []byte(`{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinition","metadata":{"name":"malformed"}}`)}},
				returnInto: true,
			},
			{returnInto: true, err: io.EOF},
		}}
		dst := &bytes.Buffer{}
		s := base(d, dst, make(chan struct{}))
		s.start(context.Background())
		if dst.Len() == 0 {
			t.Fatalf("expected output")
		}
	})

	t.Run("writes rewritten event and stops when done is closed", func(t *testing.T) {
		done := make(chan struct{})
		close(done)

		d := &fakeDecoder{results: []decodeResult{
			{
				fill:       &metav1.WatchEvent{Type: string(watch.Added), Object: runtime.RawExtension{Raw: []byte(`{"apiVersion":"prefixed.resources.group.io/v1","kind":"PrefixedSomeResource"}`)}},
				returnInto: true,
			},
		}}
		dst := &bytes.Buffer{}
		s := base(d, dst, done)
		s.start(context.Background())
		if dst.Len() == 0 {
			t.Fatalf("expected output")
		}
	})
}
