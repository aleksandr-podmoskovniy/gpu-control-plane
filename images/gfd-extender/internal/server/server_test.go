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

package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"gfd-extender/pkg/detect"
)

type fakeDetector struct {
	result []detect.Info
	err    error
}

func (f fakeDetector) DetectGPU(context.Context) ([]detect.Info, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func newTestServer(det Detector) *Server {
	return New(Config{
		ListenAddr:      "127.0.0.1:0",
		Path:            "/detect",
		ShutdownTimeout: 0,
	}, det, slogDiscardLogger())
}

func slogDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
}

func TestHandleDetectSuccess(t *testing.T) {
	detector := fakeDetector{result: []detect.Info{{Index: 1, UUID: "gpu-1"}}}
	srv := newTestServer(detector)
	req := httptest.NewRequest(http.MethodGet, "/detect", nil)
	rr := httptest.NewRecorder()

	srv.handleDetect(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	var payload []detect.Info
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unexpected response: %v", err)
	}
	if len(payload) != 1 || payload[0].UUID != "gpu-1" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestHandleDetectDetectorError(t *testing.T) {
	detector := fakeDetector{err: errors.New("boom")}
	srv := newTestServer(detector)
	req := httptest.NewRequest(http.MethodGet, "/detect", nil)
	rr := httptest.NewRecorder()

	srv.handleDetect(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestHandleDetectMethodNotAllowed(t *testing.T) {
	srv := newTestServer(fakeDetector{})
	req := httptest.NewRequest(http.MethodPost, "/detect", nil)
	rr := httptest.NewRecorder()

	srv.handleDetect(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestServerRunShutdown(t *testing.T) {
	srv := newTestServer(fakeDetector{})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- srv.Run(ctx)
	}()

	select {
	case <-srv.startedCh:
	case <-time.After(time.Second):
		t.Fatal("server did not start")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestServerRunListenError(t *testing.T) {
	srv := New(Config{
		ListenAddr: "127.0.0.1:0",
		Path:       "/detect",
	}, fakeDetector{}, slogDiscardLogger())
	srv.factory = func(string, http.Handler) httpServer {
		return &fakeHTTPServer{listenErr: errors.New("boom")}
	}
	err := srv.Run(context.Background())
	if err == nil {
		t.Fatalf("expected error when listener fails")
	}
}

func TestServerRunGracefulExit(t *testing.T) {
	srv := New(Config{
		ListenAddr: "127.0.0.1:0",
		Path:       "/detect",
	}, fakeDetector{}, slogDiscardLogger())
	srv.factory = func(string, http.Handler) httpServer {
		return &fakeHTTPServer{listenErr: http.ErrServerClosed}
	}
	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("expected graceful shutdown, got %v", err)
	}
}

func TestServerShutdownNil(t *testing.T) {
	srv := newTestServer(fakeDetector{})
	srv.httpSrv = nil
	if err := srv.shutdown(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestServerShutdownError(t *testing.T) {
	srv := newTestServer(fakeDetector{})
	srv.httpSrv = &fakeHTTPServer{shutdownErr: errors.New("boom")}
	if err := srv.shutdown(); err == nil {
		t.Fatalf("expected error from shutdown")
	}
}

type failingWriter struct {
	header http.Header
}

func (f *failingWriter) Header() http.Header {
	return f.header
}

func (f *failingWriter) WriteHeader(statusCode int) {}

func (f *failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestHandleDetectEncodeError(t *testing.T) {
	srv := newTestServer(fakeDetector{result: []detect.Info{{}}})
	writer := &failingWriter{header: make(http.Header)}
	req := httptest.NewRequest(http.MethodGet, "/detect", nil)

	srv.handleDetect(writer, req)
}

func TestHandleDetectWarningsAndMetrics(t *testing.T) {
	detectRequests.Reset()
	detectWarnings.Add(0) // ensure collector exists

	detector := fakeDetector{result: []detect.Info{{Warnings: []string{"w1", "w2"}}}}
	srv := newTestServer(detector)
	req := httptest.NewRequest(http.MethodGet, "/detect", nil)
	rr := httptest.NewRecorder()

	startWarnings := testutil.ToFloat64(detectWarnings)

	srv.handleDetect(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if diff := testutil.ToFloat64(detectWarnings) - startWarnings; diff != 2 {
		t.Fatalf("expected warnings incremented by 2, got %f", diff)
	}
	if cnt := testutil.ToFloat64(detectRequests.WithLabelValues("ok")); cnt == 0 {
		t.Fatalf("expected ok request counter incremented")
	}
}

func TestWrapMiddlewareLogsStatus(t *testing.T) {
	srv := newTestServer(fakeDetector{})
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/detect", nil)

	srv.wrapMiddleware(inner).ServeHTTP(rr, req)

	if rr.Code != http.StatusTeapot {
		t.Fatalf("expected status from inner handler, got %d", rr.Code)
	}
}

func TestMetricsEndpointExposed(t *testing.T) {
	srv := newTestServer(fakeDetector{})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler := promhttp.HandlerFor(srv.registry, promhttp.HandlerOpts{})
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected metrics 200, got %d", rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Fatalf("expected metrics body")
	}
}

func TestStdHTTPServerWrappers(t *testing.T) {
	s := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	wrapper := &stdHTTPServer{srv: s}
	errCh := make(chan error, 1)
	go func() { errCh <- wrapper.ListenAndServe() }()
	time.Sleep(50 * time.Millisecond)
	_ = wrapper.Shutdown(context.Background())
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("unexpected listen error: %v", err)
	}
}

type fakeHTTPServer struct {
	listenErr   error
	shutdownErr error
}

func (f *fakeHTTPServer) ListenAndServe() error {
	return f.listenErr
}

func (f *fakeHTTPServer) Shutdown(context.Context) error {
	return f.shutdownErr
}
