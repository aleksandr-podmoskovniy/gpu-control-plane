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
	"compress/gzip"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/tidwall/gjson"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/rewriter"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type readErrReader struct{}

func (readErrReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

type erroringResponseWriter struct {
	header http.Header
	status int
}

func (w *erroringResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *erroringResponseWriter) WriteHeader(statusCode int) { w.status = statusCode }
func (w *erroringResponseWriter) Write([]byte) (int, error)  { return 0, errors.New("write failed") }

func setSlogDebug(t *testing.T) {
	t.Helper()

	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		slog.SetDefault(old)
	})
}

func TestHandlerServeHTTP_NilReqAndNilURL(t *testing.T) {
	h := &Handler{}
	h.ServeHTTP(httptest.NewRecorder(), nil)

	h.ServeHTTP(httptest.NewRecorder(), &http.Request{Method: http.MethodGet})
}

func TestHandlerServeHTTP_TransformRequestReadError(t *testing.T) {
	h := &Handler{
		Name:         "test",
		TargetClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) { t.Fatalf("unexpected upstream call"); return nil, nil })},
		TargetURL:    &url.URL{Scheme: "http", Host: "example.com"},
		ProxyMode:    ToRenamed,
		Rewriter:     newSimpleRewriter(t),
	}
	h.Init()

	req := httptest.NewRequest(http.MethodPost, "http://example/api/v1/namespaces/ns/pods", io.NopCloser(readErrReader{}))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "can't rewrite request") {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}

func TestHandlerServeHTTP_PassResponse_Debug_NonWatch(t *testing.T) {
	setSlogDebug(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/test.group.io/v1/virtualmachines" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	}))
	t.Cleanup(upstream.Close)

	u, _ := url.Parse(upstream.URL)

	h := &Handler{
		Name:            "test",
		TargetClient:    upstream.Client(),
		TargetURL:       u,
		ProxyMode:       ToOriginal,
		Rewriter:        newSimpleRewriter(t),
		MetricsProvider: NewMetricsProvider(),
	}
	h.Init()

	req := httptest.NewRequest(http.MethodGet, "http://example/apis/test.group.io/v1/virtualmachines", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "pong" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}

func TestHandlerServeHTTP_PassResponse_Debug_Watch(t *testing.T) {
	setSlogDebug(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/test.group.io/v1/virtualmachines" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.RawQuery != "watch=true" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event"))
	}))
	t.Cleanup(upstream.Close)

	u, _ := url.Parse(upstream.URL)

	h := &Handler{
		Name:            "test",
		TargetClient:    upstream.Client(),
		TargetURL:       u,
		ProxyMode:       ToOriginal,
		Rewriter:        newSimpleRewriter(t),
		MetricsProvider: NewMetricsProvider(),
	}
	h.Init()

	req := httptest.NewRequest(http.MethodGet, "http://example/apis/test.group.io/v1/virtualmachines?watch=true", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "event" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}

func TestHandlerServeHTTP_NoRewriteRequestBodyIsForwarded(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "reqbody" {
			t.Fatalf("unexpected request body: %q", string(body))
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)

	u, _ := url.Parse(upstream.URL)

	h := &Handler{
		Name:            "test",
		TargetClient:    upstream.Client(),
		TargetURL:       u,
		ProxyMode:       ToOriginal,
		Rewriter:        newSimpleRewriter(t),
		MetricsProvider: NewMetricsProvider(),
	}
	h.Init()

	req := httptest.NewRequest(http.MethodPost, "http://example/apis/test.group.io/v1/virtualmachines", bytes.NewBufferString("reqbody"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandlerServeHTTP_RewriteRequestAndResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/prefixed.resources.group.io/v1/namespaces/ns/prefixedsomeresources" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if gjson.GetBytes(body, "apiVersion").String() != "prefixed.resources.group.io/v1" {
			t.Fatalf("expected renamed apiVersion, got: %s", gjson.GetBytes(body, "apiVersion").Raw)
		}
		if gjson.GetBytes(body, "kind").String() != "PrefixedSomeResource" {
			t.Fatalf("expected renamed kind, got: %s", gjson.GetBytes(body, "kind").Raw)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"kind":"PrefixedSomeResource","apiVersion":"prefixed.resources.group.io/v1","metadata":{"name":"sr1"}}`))
	}))
	t.Cleanup(upstream.Close)

	u, _ := url.Parse(upstream.URL)

	h := &Handler{
		Name:            "test",
		TargetClient:    upstream.Client(),
		TargetURL:       u,
		ProxyMode:       ToRenamed,
		Rewriter:        newSimpleRewriter(t),
		MetricsProvider: NewMetricsProvider(),
	}
	h.Init()

	req := httptest.NewRequest(http.MethodPost,
		"http://example/apis/original.group.io/v1/namespaces/ns/someresources",
		bytes.NewBufferString(`{"kind":"SomeResource","apiVersion":"original.group.io/v1","metadata":{"name":"sr1","namespace":"ns"}}`),
	)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gjson.GetBytes(rr.Body.Bytes(), "apiVersion").String() != "original.group.io/v1" {
		t.Fatalf("expected restored apiVersion, got: %s", gjson.GetBytes(rr.Body.Bytes(), "apiVersion").Raw)
	}
	if gjson.GetBytes(rr.Body.Bytes(), "kind").String() != "SomeResource" {
		t.Fatalf("expected restored kind, got: %s", gjson.GetBytes(rr.Body.Bytes(), "kind").Raw)
	}
}

func TestHandlerServeHTTP_PatchRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/prefixed.resources.group.io/v1/namespaces/ns/prefixedsomeresources/sr1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPatch {
			t.Fatalf("expected PATCH, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if !gjson.ValidBytes(body) {
			t.Fatalf("expected valid json patch")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"kind":"PrefixedSomeResource","apiVersion":"prefixed.resources.group.io/v1","metadata":{"name":"sr1"}}`))
	}))
	t.Cleanup(upstream.Close)

	u, _ := url.Parse(upstream.URL)

	h := &Handler{
		Name:            "test",
		TargetClient:    upstream.Client(),
		TargetURL:       u,
		ProxyMode:       ToRenamed,
		Rewriter:        newSimpleRewriter(t),
		MetricsProvider: NewMetricsProvider(),
	}
	h.Init()

	req := httptest.NewRequest(http.MethodPatch,
		"http://example/apis/original.group.io/v1/namespaces/ns/someresources/sr1",
		bytes.NewBufferString(`{"metadata":{"labels":{"a":"b"}}}`),
	)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandlerServeHTTP_RewriteStream_SuccessAndError(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/pods" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if r.URL.RawQuery != "watch=true" {
				t.Fatalf("unexpected query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"type":"ADDED","object":{"apiVersion":"v1","kind":"Pod","metadata":{"name":"p1"}}}`))
		}))
		t.Cleanup(upstream.Close)

		u, _ := url.Parse(upstream.URL)

		h := &Handler{
			Name:            "test",
			TargetClient:    upstream.Client(),
			TargetURL:       u,
			ProxyMode:       ToOriginal,
			Rewriter:        newSimpleRewriter(t),
			MetricsProvider: NewMetricsProvider(),
		}
		h.Init()

		req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/pods?watch=true", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if rr.Body.Len() == 0 {
			t.Fatalf("expected non-empty stream output")
		}
	})

	t.Run("error", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("nope"))
		}))
		t.Cleanup(upstream.Close)

		u, _ := url.Parse(upstream.URL)

		h := &Handler{
			Name:            "test",
			TargetClient:    upstream.Client(),
			TargetURL:       u,
			ProxyMode:       ToOriginal,
			Rewriter:        newSimpleRewriter(t),
			MetricsProvider: NewMetricsProvider(),
		}
		h.Init()

		req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/pods?watch=true", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "watch stream") {
			t.Fatalf("unexpected body: %q", rr.Body.String())
		}
	})
}

func TestHandlerServeHTTP_MuteSwitchCases(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1"}`))
	}))
	t.Cleanup(upstream.Close)

	u, _ := url.Parse(upstream.URL)

	h := &Handler{
		Name:            "test",
		TargetClient:    upstream.Client(),
		TargetURL:       u,
		ProxyMode:       ToOriginal,
		Rewriter:        newSimpleRewriter(t),
		MetricsProvider: NewMetricsProvider(),
	}
	h.Init()

	cases := []string{
		// leases -> isMute = true
		"http://example/apis/coordination.k8s.io/v1/namespaces/ns/leases",
		// endpoints -> isMute = true
		"http://example/api/v1/namespaces/ns/endpoints",
		// clusterrolebindings -> isMute = false
		"http://example/apis/rbac.authorization.k8s.io/v1/clusterrolebindings",
		// clustervirtualmachineimages -> isMute = false
		"http://example/apis/test.group.io/v1/clustervirtualmachineimages",
		// virtualmachines/status -> isMute = false
		"http://example/apis/test.group.io/v1/namespaces/ns/virtualmachines/vm1/status",
	}

	for _, uStr := range cases {
		req := httptest.NewRequest(http.MethodGet, uStr, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("url=%s: expected 200, got %d", uStr, rr.Code)
		}
	}
}

func TestHandler_transformRequest_Branches(t *testing.T) {
	h := &Handler{
		ProxyMode: ToRenamed,
		Rewriter:  newSimpleRewriter(t),
	}

	t.Run("nil request returns error", func(t *testing.T) {
		_, _, err := h.transformRequest(nil, nil)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("read body error", func(t *testing.T) {
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api"},
			Body:   io.NopCloser(readErrReader{}),
			Header: make(http.Header),
		}
		_, _, err := h.transformRequest(nil, req)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("rewrite JSON payload and fix Content-Length + Accept", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost,
			"http://example/apis/original.group.io/v1/namespaces/ns/someresources",
			bytes.NewBufferString(`{"kind":"SomeResource","apiVersion":"original.group.io/v1","metadata":{"name":"sr1"}}`),
		)
		req.Header.Set("Content-Length", "999")
		req.Header.Add("Accept", "application/vnd.kubernetes.protobuf;g=meta.k8s.io;v=v1;as=Table")
		req.Header.Add("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1")
		req.Header.Add("Accept", "application/yaml")

		targetReq := rewriter.NewTargetRequest(h.Rewriter, req)

		orig, rwr, err := h.transformRequest(targetReq, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(orig) == 0 || len(rwr) == 0 {
			t.Fatalf("expected non-empty orig and rewritten bytes")
		}
		if req.ContentLength != int64(len(rwr)) {
			t.Fatalf("expected ContentLength=%d, got %d", len(rwr), req.ContentLength)
		}
		if req.Header.Get("Content-Length") != strconv.Itoa(len(rwr)) {
			t.Fatalf("expected Content-Length=%d, got %q", len(rwr), req.Header.Get("Content-Length"))
		}
		if got := req.Header.Values("Accept"); len(got) != 3 || got[0] != "application/json" || got[1] != "application/json" || got[2] != "application/yaml" {
			t.Fatalf("unexpected Accept values: %#v", got)
		}
		if req.URL.Path != targetReq.Path() {
			t.Fatalf("expected path rewritten to %s, got %s", targetReq.Path(), req.URL.Path)
		}
	})

	t.Run("rewrite JSON payload error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost,
			"http://example/apis/apiextensions.k8s.io/v1/customresourcedefinitions",
			bytes.NewBufferString(`{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinition","metadata":{"name":"malformed"}}`),
		)
		targetReq := rewriter.NewTargetRequest(h.Rewriter, req)
		_, _, err := h.transformRequest(targetReq, req)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("rewrite PATCH payload", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch,
			"http://example/apis/original.group.io/v1/namespaces/ns/someresources/sr1",
			bytes.NewBufferString(`{"metadata":{"labels":{"a":"b"}}}`),
		)
		req.Header.Set("Content-Length", "999")
		targetReq := rewriter.NewTargetRequest(h.Rewriter, req)

		_, rwr, err := h.transformRequest(targetReq, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rwr) == 0 {
			t.Fatalf("expected non-empty rewritten patch")
		}
		if req.Header.Get("Content-Length") != strconv.Itoa(len(rwr)) {
			t.Fatalf("expected Content-Length=%d, got %q", len(rwr), req.Header.Get("Content-Length"))
		}
	})

	t.Run("force Accept JSON for core watches", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/pods?watch=true", nil)
		targetReq := rewriter.NewTargetRequest(h.Rewriter, req)

		_, _, err := h.transformRequest(targetReq, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := req.Header.Values("Accept"); len(got) != 1 || got[0] != "application/json" {
			t.Fatalf("expected forced Accept application/json, got %#v", got)
		}
	})
}

func TestHandler_passResponse_CopyErrorBranch(t *testing.T) {
	h := &Handler{MetricsProvider: NewMetricsProvider()}
	rwr := newSimpleRewriter(t)

	req := httptest.NewRequest(http.MethodGet, "http://example/apis/test.group.io/v1/virtualmachines", nil)
	targetReq := rewriter.NewTargetRequest(rwr, req)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(bytes.NewBufferString("payload")),
	}

	w := &erroringResponseWriter{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h.passResponse(context.Background(), targetReq, w, resp, logger)

	if w.status != http.StatusOK {
		t.Fatalf("expected status=200, got %d", w.status)
	}
}

func TestHandler_transformResponse_Branches(t *testing.T) {
	t.Run("gzip decode error", func(t *testing.T) {
		h := &Handler{ProxyMode: ToOriginal, Rewriter: newSimpleRewriter(t), MetricsProvider: NewMetricsProvider()}
		targetReq := rewriter.NewTargetRequest(h.Rewriter, httptest.NewRequest(http.MethodGet, "http://example/api/v1/pods", nil))

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Encoding": []string{"gzip"}},
			Body:       io.NopCloser(bytes.NewBufferString("not-gzip")),
		}
		rr := httptest.NewRecorder()
		h.transformResponse(context.Background(), targetReq, rr, resp, slog.New(slog.NewTextHandler(io.Discard, nil)))

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rr.Code)
		}
	})

	t.Run("read payload error", func(t *testing.T) {
		h := &Handler{ProxyMode: ToOriginal, Rewriter: newSimpleRewriter(t), MetricsProvider: NewMetricsProvider()}
		targetReq := rewriter.NewTargetRequest(h.Rewriter, httptest.NewRequest(http.MethodGet, "http://example/api/v1/pods", nil))

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(readErrReader{}),
		}
		rr := httptest.NewRecorder()
		h.transformResponse(context.Background(), targetReq, rr, resp, slog.New(slog.NewTextHandler(io.Discard, nil)))

		if rr.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d", rr.Code)
		}
	})

	t.Run("invalid JSON with application/json content-type is passed", func(t *testing.T) {
		h := &Handler{ProxyMode: ToOriginal, Rewriter: newSimpleRewriter(t), MetricsProvider: NewMetricsProvider()}
		targetReq := rewriter.NewTargetRequest(h.Rewriter, httptest.NewRequest(http.MethodGet, "http://example/api/v1/pods", nil))

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString("{not json")),
		}
		rr := httptest.NewRecorder()
		h.transformResponse(context.Background(), targetReq, rr, resp, slog.New(slog.NewTextHandler(io.Discard, nil)))

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if rr.Body.String() != "{not json" {
			t.Fatalf("unexpected body: %q", rr.Body.String())
		}
	})

	t.Run("non-JSON content-type is passed", func(t *testing.T) {
		h := &Handler{ProxyMode: ToOriginal, Rewriter: newSimpleRewriter(t), MetricsProvider: NewMetricsProvider()}
		targetReq := rewriter.NewTargetRequest(h.Rewriter, httptest.NewRequest(http.MethodGet, "http://example/api/v1/pods", nil))

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(bytes.NewBufferString("hello")),
		}
		rr := httptest.NewRecorder()
		h.transformResponse(context.Background(), targetReq, rr, resp, slog.New(slog.NewTextHandler(io.Discard, nil)))

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if rr.Body.String() != "hello" {
			t.Fatalf("unexpected body: %q", rr.Body.String())
		}
	})

	t.Run("invalid JSON with gzip encoding drops Content-Encoding", func(t *testing.T) {
		h := &Handler{ProxyMode: ToOriginal, Rewriter: newSimpleRewriter(t), MetricsProvider: NewMetricsProvider()}
		targetReq := rewriter.NewTargetRequest(h.Rewriter, httptest.NewRequest(http.MethodGet, "http://example/api/v1/pods", nil))

		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write([]byte("{not json"))
		_ = zw.Close()

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":     []string{"application/json"},
				"Content-Encoding": []string{"gzip"},
			},
			Body: io.NopCloser(bytes.NewReader(buf.Bytes())),
		}
		rr := httptest.NewRecorder()
		h.transformResponse(context.Background(), targetReq, rr, resp, slog.New(slog.NewTextHandler(io.Discard, nil)))

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if rr.Body.String() != "{not json" {
			t.Fatalf("unexpected body: %q", rr.Body.String())
		}
		if got := rr.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("expected Content-Encoding to be removed, got %q", got)
		}
	})

	t.Run("non-SkipItem rewrite error returns 500", func(t *testing.T) {
		h := &Handler{ProxyMode: ToOriginal, Rewriter: newSimpleRewriter(t), MetricsProvider: NewMetricsProvider()}
		targetReq := rewriter.NewTargetRequest(h.Rewriter, httptest.NewRequest(http.MethodGet, "http://example/apis/apiextensions.k8s.io/v1/customresourcedefinitions", nil))

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinition","metadata":{"name":"malformed"}}`)),
		}
		rr := httptest.NewRecorder()
		h.transformResponse(context.Background(), targetReq, rr, resp, slog.New(slog.NewTextHandler(io.Discard, nil)))

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rr.Code)
		}
	})

	t.Run("SkipItem is returned as NotFound", func(t *testing.T) {
		h := &Handler{ProxyMode: ToRenamed, Rewriter: newSimpleRewriter(t), MetricsProvider: NewMetricsProvider()}
		req := httptest.NewRequest(http.MethodGet, "http://example/apis/original.group.io/v1/namespaces/ns/someresources/sr1", nil)
		targetReq := rewriter.NewTargetRequest(h.Rewriter, req)

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"kind":"SomeResource","apiVersion":"original.group.io/v1","metadata":{"name":"sr1"}}`)),
		}
		rr := httptest.NewRecorder()
		h.transformResponse(context.Background(), targetReq, rr, resp, slog.New(slog.NewTextHandler(io.Discard, nil)))

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rr.Code)
		}
		if gjson.GetBytes(rr.Body.Bytes(), "reason").String() != "NotFound" {
			t.Fatalf("expected NotFound response, got: %q", rr.Body.String())
		}
	})

	t.Run("write error is handled", func(t *testing.T) {
		h := &Handler{ProxyMode: ToOriginal, Rewriter: newSimpleRewriter(t), MetricsProvider: NewMetricsProvider()}
		targetReq := rewriter.NewTargetRequest(h.Rewriter, httptest.NewRequest(http.MethodGet, "http://example/api/v1/pods", nil))

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"kind":"ConfigMap","apiVersion":"v1"}`)),
		}

		w := &erroringResponseWriter{}
		h.transformResponse(context.Background(), targetReq, w, resp, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if w.status != http.StatusOK {
			t.Fatalf("expected status=200, got %d", w.status)
		}
	})

	t.Run("webhook responses call webhook log branch", func(t *testing.T) {
		rwr := newSimpleRewriter(t)
		rwr.Rules.Webhooks = map[string]rewriter.WebhookRule{
			"/webhook": {Path: "/webhook", Group: "original.group.io", Resource: "someresources"},
		}

		h := &Handler{ProxyMode: ToOriginal, Rewriter: rwr, MetricsProvider: NewMetricsProvider()}

		req := httptest.NewRequest(http.MethodPost, "http://example/webhook", nil)
		targetReq := rewriter.NewTargetRequest(rwr, req)
		if targetReq == nil || !targetReq.IsWebhook() {
			t.Fatalf("expected webhook target request")
		}

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"kind":"SomeResource","apiVersion":"original.group.io/v1","metadata":{"name":"sr1"}}`)),
		}
		rr := httptest.NewRecorder()
		h.transformResponse(context.Background(), targetReq, rr, resp, slog.New(slog.NewTextHandler(io.Discard, nil)))

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})
}
